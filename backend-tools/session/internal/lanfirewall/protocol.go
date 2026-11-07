// Package lanfirewall validates the concrete offline LAN firewall protocol.
// Parsing or validating these values establishes neither caller authentication,
// protected-store ownership, user consent nor native firewall state.
package lanfirewall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const RequestBudget = 16 << 10
const ReceiptBudget = 32 << 10
const MaxSequence = 1<<53 - 1

type Scope struct {
	Protocol       string   `json:"protocol"`
	LocalAddress   string   `json:"localAddress"`
	LocalPort      uint16   `json:"localPort"`
	InterfaceGUID  string   `json:"interfaceGuid"`
	InterfaceLUID  string   `json:"interfaceLuid"`
	Profile        string   `json:"profile"`
	ClientNetworks []string `json:"clientNetworks"`
	WorkerSHA256   string   `json:"workerSha256"`
}
type Request struct {
	Version            int    `json:"version"`
	Operation          string `json:"operation"`
	Installation       string `json:"installation"`
	UserSID            string `json:"userSid"`
	Transaction        string `json:"transaction"`
	Nonce              string `json:"nonce"`
	ExpectedSequence   uint64 `json:"expectedSequence"`
	ReviewSHA256       string `json:"reviewSha256"`
	CallerPID          uint32 `json:"callerPid"`
	CallerImageSHA256  string `json:"callerImageSha256"`
	CallerCreationTime string `json:"callerCreationTime"`
	IssuedAt           string `json:"issuedAt"`
	ExpiresAt          string `json:"expiresAt"`
	Scope              Scope  `json:"scope"`
}
type WorkerBinding struct {
	Path           string `json:"path"`
	FileID         string `json:"fileId"`
	VolumeSerial   string `json:"volumeSerial"`
	SHA256         string `json:"sha256"`
	InterfaceAlias string `json:"interfaceAlias"`
}

// CallerIdentity must come from native pipe/process/token/file observations,
// never another JSON field or a caller-provided signature claim.
type CallerIdentity struct {
	UserSID      string
	PID          uint32
	CreationTime string
	ImageSHA256  string
}
type Receipt struct {
	Version           int           `json:"version"`
	Request           Request       `json:"request"`
	Sequence          uint64        `json:"sequence"`
	State             string        `json:"state"`
	CreatedAt         string        `json:"createdAt"`
	UpdatedAt         string        `json:"updatedAt"`
	RuleName          string        `json:"ruleName"`
	ScopeSHA256       string        `json:"scopeSha256"`
	NativeScopeSHA256 string        `json:"nativeScopeSha256"`
	Worker            WorkerBinding `json:"worker"`
}

var guid = regexp.MustCompile(`^\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}$`)

func hexBytes(text string, length int) bool {
	decoded, err := hex.DecodeString(text)
	return err == nil && len(decoded) == length && hex.EncodeToString(decoded) == text
}
func decimal(text string) bool {
	value, err := strconv.ParseUint(text, 10, 64)
	return err == nil && value != 0 && strconv.FormatUint(value, 10) == text
}
func ordinarySID(text string) bool {
	parts := strings.Split(text, "-")
	if len(parts) != 8 || parts[0] != "S" || parts[1] != "1" || !(parts[2] == "5" && parts[3] == "21" || parts[2] == "12" && parts[3] == "1") {
		return false
	}
	for _, part := range parts[2:] {
		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil || strconv.FormatUint(value, 10) != part {
			return false
		}
	}
	return true
}
func moment(text string) (time.Time, error) {
	value, err := time.Parse(time.RFC3339Nano, text)
	if err != nil || value.IsZero() {
		return time.Time{}, errors.New("firewall protocol timestamp is invalid")
	}
	return value, nil
}
func boundedText(text string, max int) bool {
	return len(text) > 0 && len(text) <= max && utf8.ValidString(text) && !strings.ContainsFunc(text, unicode.IsControl)
}

func (scope Scope) Validate() error {
	if (scope.Protocol != "tcp" && scope.Protocol != "udp") || scope.LocalPort == 0 || !guid.MatchString(scope.InterfaceGUID) || scope.InterfaceGUID == "{00000000-0000-0000-0000-000000000000}" || !decimal(scope.InterfaceLUID) || !hexBytes(scope.WorkerSHA256, 32) || (scope.Profile != "private" && scope.Profile != "domain") {
		return errors.New("exact LAN port/protocol/interface/profile/worker scope required")
	}
	address, err := netip.ParseAddr(scope.LocalAddress)
	if err != nil || !address.Is4() || !address.IsPrivate() || address.String() != scope.LocalAddress {
		return errors.New("LAN firewall scope requires one private IPv4 address")
	}
	if len(scope.ClientNetworks) == 0 || len(scope.ClientNetworks) > 8 {
		return errors.New("LAN firewall scope requires one to eight client CIDRs")
	}
	last := ""
	for _, raw := range scope.ClientNetworks {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || !prefix.Addr().Is4() || prefix.Masked() != prefix || prefix.String() != raw || raw <= last {
			return errors.New("LAN client CIDRs must be sorted distinct canonical IPv4 prefixes")
		}
		contained := false
		for _, rawBlock := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
			block := netip.MustParsePrefix(rawBlock)
			if block.Contains(prefix.Addr()) && prefix.Bits() >= block.Bits() {
				contained = true
			}
		}
		if !contained {
			return errors.New("LAN client CIDR extends outside private IPv4 scope")
		}
		last = raw
	}
	return nil
}
func digest(prefix string, value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(append([]byte(prefix+"\n"), data...))
	return hex.EncodeToString(sum[:])
}
func (scope Scope) Digest() string     { return digest("orbwin-lan-firewall-scope-v1", scope) }
func (request Request) Digest() string { return digest("orbwin-lan-firewall-request-v1", request) }
func (request Request) RuleName() string {
	if !hexBytes(request.Installation, 16) || !hexBytes(request.ReviewSHA256, 32) {
		return ""
	}
	return "OrbWin-LAN-" + request.Installation + "-" + request.ReviewSHA256[:16]
}

func (request Request) validate() error {
	if request.Version != 1 || (request.Operation != "inspect" && request.Operation != "create" && request.Operation != "remove") || !hexBytes(request.Installation, 16) || !ordinarySID(request.UserSID) || !hexBytes(request.Transaction, 16) || !hexBytes(request.Nonce, 32) || request.ExpectedSequence > MaxSequence || !hexBytes(request.ReviewSHA256, 32) || request.CallerPID == 0 || !hexBytes(request.CallerImageSHA256, 32) || !decimal(request.CallerCreationTime) {
		return errors.New("unsupported or malformed fixed firewall request identity")
	}
	issued, err := moment(request.IssuedAt)
	if err != nil {
		return err
	}
	expires, err := moment(request.ExpiresAt)
	if err != nil || !expires.After(issued) || expires.Sub(issued) > 5*time.Minute {
		return errors.New("firewall request lifetime must be positive and at most five minutes")
	}
	if request.Operation != "inspect" && request.ExpectedSequence > MaxSequence-2 {
		return errors.New("firewall receipt sequence lacks intent/acknowledgement headroom")
	}
	return request.Scope.Validate()
}
func (request Request) Validate(now time.Time) error {
	if err := request.validate(); err != nil {
		return err
	}
	issued, _ := moment(request.IssuedAt)
	expires, _ := moment(request.ExpiresAt)
	if now.Before(issued) || !now.Before(expires) {
		return errors.New("firewall request is future-dated or expired; review again")
	}
	return nil
}

func GuardCaller(request Request, actual CallerIdentity, now time.Time) error {
	if err := request.Validate(now); err != nil {
		return err
	}
	if actual.UserSID != request.UserSID || actual.PID != request.CallerPID || actual.CreationTime != request.CallerCreationTime || actual.ImageSHA256 != request.CallerImageSHA256 {
		return errors.New("firewall request differs from native caller process/token/image identity")
	}
	return nil
}

// GuardStoredOwner assumes a separately verified admin-protected receipt read.
// It does not authenticate a parsed user-home receipt or authorize a mutation.
func GuardStoredOwner(request Request, receipt *Receipt) error {
	if err := request.validate(); err != nil {
		return err
	}
	if receipt == nil {
		if request.ExpectedSequence != 0 || request.Operation == "remove" {
			return errors.New("firewall request has no owned receipt")
		}
		return nil
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	if request.Installation != receipt.Request.Installation || request.UserSID != receipt.Request.UserSID || request.ReviewSHA256 != receipt.Request.ReviewSHA256 || request.Scope.Digest() != receipt.ScopeSHA256 || request.Operation != "inspect" && request.ExpectedSequence != receipt.Sequence {
		return errors.New("firewall request differs from protected receipt ownership/scope/sequence")
	}
	return nil
}

func (worker WorkerBinding) validate(scope Scope) error {
	path := worker.Path
	if !boundedText(path, 512) || len(path) < 4 || path[0] < 'A' || path[0] > 'Z' || path[1:3] != ":\\" || strings.ContainsAny(path[2:], ":/<>|?*\"") || !strings.HasSuffix(path, "\\orbwin-session.exe") || !hexBytes(worker.FileID, 16) || !decimal(worker.VolumeSerial) || worker.SHA256 != scope.WorkerSHA256 || !boundedText(worker.InterfaceAlias, 1024) {
		return errors.New("receipt requires exact native-resolved worker/interface metadata")
	}
	for _, part := range strings.Split(path[3:], "\\") {
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		reserved := base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9'
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || reserved {
			return errors.New("unsupported native worker path spelling")
		}
	}
	return nil
}
func (receipt Receipt) Validate() error {
	if receipt.Version != 1 || receipt.Sequence == 0 || receipt.Sequence > MaxSequence {
		return errors.New("unsupported firewall receipt version/sequence")
	}
	if err := receipt.Request.validate(); err != nil {
		return err
	}
	if err := receipt.Worker.validate(receipt.Request.Scope); err != nil {
		return err
	}
	created, err := moment(receipt.CreatedAt)
	if err != nil {
		return err
	}
	updated, err := moment(receipt.UpdatedAt)
	if err != nil || updated.Before(created) {
		return errors.New("firewall receipt times regress")
	}
	issued, _ := moment(receipt.Request.IssuedAt)
	if updated.Before(issued) {
		return errors.New("firewall receipt acknowledgement predates its request")
	}
	if receipt.RuleName != receipt.Request.RuleName() || receipt.ScopeSHA256 != receipt.Request.Scope.Digest() {
		return errors.New("firewall receipt rule/scope differs from retained request")
	}
	switch receipt.State {
	case "create-intent":
		if receipt.Request.Operation != "create" || receipt.NativeScopeSHA256 != "" {
			return errors.New("create intent cannot claim native success")
		}
	case "present":
		if receipt.Request.Operation != "create" || receipt.NativeScopeSHA256 != receipt.ScopeSHA256 {
			return errors.New("present receipt requires matching normalized native scope")
		}
	case "remove-intent":
		if receipt.Request.Operation != "remove" || receipt.NativeScopeSHA256 != "" && receipt.NativeScopeSHA256 != receipt.ScopeSHA256 {
			return errors.New("remove intent native scope is invalid")
		}
	case "removed":
		if receipt.Request.Operation != "remove" || receipt.NativeScopeSHA256 != "" {
			return errors.New("removed receipt must acknowledge native absence")
		}
	default:
		return errors.New("unknown firewall receipt state")
	}
	delta := uint64(1)
	if receipt.State == "present" || receipt.State == "removed" {
		delta = 2
	}
	if receipt.Sequence != receipt.Request.ExpectedSequence+delta {
		return errors.New("firewall receipt sequence differs from its accepted request")
	}
	return nil
}

// ValidateNext checks receipt consistency only; a native provider/protected store
// must establish actual observations, authenticated consent and write ordering.
func ValidateNext(previous *Receipt, next Receipt) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if previous == nil {
		if next.Sequence != 1 || next.State != "create-intent" || next.Request.ExpectedSequence != 0 {
			return errors.New("first firewall receipt must be a flushed create intent")
		}
		return nil
	}
	if err := previous.Validate(); err != nil {
		return err
	}
	if previous.Sequence == MaxSequence || next.Sequence != previous.Sequence+1 || next.CreatedAt != previous.CreatedAt || next.Request.Installation != previous.Request.Installation || next.Request.UserSID != previous.Request.UserSID || next.Request.ReviewSHA256 != previous.Request.ReviewSHA256 || next.ScopeSHA256 != previous.ScopeSHA256 || next.Worker != previous.Worker {
		return errors.New("firewall receipt ownership/scope/sequence changed")
	}
	before, _ := moment(previous.UpdatedAt)
	after, _ := moment(next.UpdatedAt)
	if after.Before(before) {
		return errors.New("firewall receipt time regressed")
	}
	ack := previous.State == "create-intent" && next.State == "present" || previous.State == "remove-intent" && next.State == "removed"
	if ack {
		if next.Request.Digest() != previous.Request.Digest() {
			return errors.New("firewall acknowledgement differs from accepted intent")
		}
		return nil
	}
	intent := (previous.State == "present" || previous.State == "create-intent") && next.State == "remove-intent" || previous.State == "removed" && next.State == "create-intent"
	if !intent || next.Request.ExpectedSequence != previous.Sequence || next.Request.Transaction == previous.Request.Transaction || next.Request.Nonce == previous.Request.Nonce {
		return errors.New("unsupported or replayed firewall receipt transition")
	}
	return nil
}

type jsonSchema map[string]jsonSchema

var scopeSchema = jsonSchema{"protocol": nil, "localAddress": nil, "localPort": nil, "interfaceGuid": nil, "interfaceLuid": nil, "profile": nil, "clientNetworks": nil, "workerSha256": nil}
var requestSchema = jsonSchema{"version": nil, "operation": nil, "installation": nil, "userSid": nil, "transaction": nil, "nonce": nil, "expectedSequence": nil, "reviewSha256": nil, "callerPid": nil, "callerImageSha256": nil, "callerCreationTime": nil, "issuedAt": nil, "expiresAt": nil, "scope": scopeSchema}
var receiptSchema = jsonSchema{"version": nil, "request": requestSchema, "sequence": nil, "state": nil, "createdAt": nil, "updatedAt": nil, "ruleName": nil, "scopeSha256": nil, "nativeScopeSha256": nil, "worker": {"path": nil, "fileId": nil, "volumeSerial": nil, "sha256": nil, "interfaceAlias": nil}}

func checkObject(decoder *json.Decoder, schema jsonSchema) error {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("firewall protocol object required")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		child, known := schema[key]
		if !ok || !known || seen[key] {
			return errors.New("unknown, case-aliased or duplicate firewall protocol field")
		}
		seen[key] = true
		if child != nil {
			if err := checkObject(decoder, child); err != nil {
				return err
			}
		} else {
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				return err
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if len(seen) != len(schema) {
		return errors.New("firewall protocol is missing required fields")
	}
	return nil
}
func decode(data []byte, budget int, schema jsonSchema, value any) error {
	if len(data) == 0 || len(data) > budget || !utf8.Valid(data) {
		return errors.New("firewall protocol exceeds its byte/encoding budget")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := checkObject(decoder, schema); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("unexpected firewall protocol suffix")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
func ParseRequest(data []byte, now time.Time) (Request, error) {
	var request Request
	if err := decode(data, RequestBudget, requestSchema, &request); err != nil {
		return Request{}, err
	}
	if err := request.Validate(now); err != nil {
		return Request{}, err
	}
	return request, nil
}
func ParseReceipt(data []byte) (Receipt, error) {
	var receipt Receipt
	if err := decode(data, ReceiptBudget, receiptSchema, &receipt); err != nil {
		return Receipt{}, err
	}
	if err := receipt.Validate(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}
