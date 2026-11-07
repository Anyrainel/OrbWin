// Package lanhelper defines the bounded LAN v2 transport only. Parsing a frame
// is not native authentication, protected-store proof or firewall authority.
// The original lanfirewall v1 wire and digest semantics remain unchanged.
package lanhelper

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
	"reflect"
	"strings"
	"time"
)

const FrameBudget = 16 << 10
const MaxStreams = 16
const ObservationTTL = 5 * time.Second
const HeartbeatInterval = time.Second

type StageRequest struct {
	Base        lanfirewall.Request  `json:"base"`
	Publication lanlease.Publication `json:"publication"`
}
type Prepared struct {
	Spec     lanlease.Spec `json:"spec"`
	LeaseSHA string        `json:"leaseSha256"`
	Sequence uint64        `json:"sequence"`
}
type Child struct {
	Identity lanfirewall.CallerIdentity `json:"identity"`
}
type Grant struct {
	Observation lanlease.Observation         `json:"observation"`
	Rule        lanlease.RuleAcknowledgement `json:"rule"`
}
type Stream struct {
	ID   uint64 `json:"id"`
	Peer string `json:"peer,omitempty"`
}

// Exactly one body is allowed for its fixed tag. There is no command, endpoint,
// caller-chosen program, store, shell, argument-vector or secret field.
type Frame struct {
	Version     int                   `json:"version"`
	Kind        string                `json:"kind"`
	Sequence    uint64                `json:"sequence"`
	LeaseSHA    string                `json:"leaseSha256,omitempty"`
	Stage       *StageRequest         `json:"stage,omitempty"`
	Prepared    *Prepared             `json:"prepared,omitempty"`
	Child       *Child                `json:"child,omitempty"`
	Grant       *Grant                `json:"grant,omitempty"`
	Observation *lanlease.Observation `json:"observation,omitempty"`
	Stream      *Stream               `json:"stream,omitempty"`
}

func ValidSHA(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func ValidateStage(stage StageRequest, actual lanfirewall.CallerIdentity, now time.Time) error {
	if stage.Base.Operation != "create" || stage.Base.ExpectedSequence != 0 {
		return errors.New("LAN v2 requires a fresh fixed create review")
	}
	if e := lanfirewall.GuardCaller(stage.Base, actual, now); e != nil {
		return e
	}
	p := stage.Publication
	started, e := time.Parse(time.RFC3339Nano, p.StartedAt)
	if !recordID(p.OwnerInstance) || !ValidSHA(p.ContainerID) || p.Daemon == "" || len(p.Daemon) > 128 || e != nil || started.IsZero() || p.Generation == 0 || p.HostPort != stage.Base.Scope.LocalPort || p.ContainerPort == 0 || p.Protocol != stage.Base.Scope.Protocol {
		return errors.New("exact upstream publication generation/incarnation tuple unavailable")
	}
	return nil
}
func PipeName(installation, lease string) (string, error) {
	for _, s := range []string{installation, lease} {
		b, e := hex.DecodeString(s)
		if e != nil || len(b) != 16 || hex.EncodeToString(b) != s {
			return "", errors.New("canonical fixed LAN pipe identity required")
		}
	}
	return `\\.\pipe\OrbWinLAN-v2-` + installation + "-" + lease, nil
}
func (f Frame) Validate() error {
	if f.Version != 2 || f.Sequence == 0 || f.Sequence > lanfirewall.MaxSequence {
		return errors.New("unsupported LAN v2 version/sequence")
	}
	count := 0
	for _, present := range []bool{f.Stage != nil, f.Prepared != nil, f.Child != nil, f.Grant != nil, f.Observation != nil, f.Stream != nil} {
		if present {
			count++
		}
	}
	if f.Kind != "stage-request" && !ValidSHA(f.LeaseSHA) {
		return errors.New("fixed lease digest required")
	}
	switch f.Kind {
	case "stage-request":
		if count != 1 || f.Stage == nil || f.LeaseSHA != "" || f.Sequence != 1 || f.Stage.Base.Operation != "create" || f.Stage.Base.ExpectedSequence != 0 {
			return errors.New("LAN stage request shape differs")
		}
	case "prepared":
		if count != 1 || f.Prepared == nil || f.Prepared.LeaseSHA != f.LeaseSHA || f.Prepared.Sequence != f.Sequence {
			return errors.New("LAN prepared reply shape differs")
		}
	case "child-ack", "worker-hello":
		if count != 1 || f.Child == nil || f.Child.Identity.PID == 0 {
			return errors.New("LAN child identity shape differs")
		}
	case "commit", "worker-grant":
		if count != 1 || f.Grant == nil || f.Grant.Rule.LeaseSHA != f.LeaseSHA {
			return errors.New("LAN committed grant shape differs")
		}
	case "refresh":
		if count != 1 || f.Observation == nil {
			return errors.New("LAN observation shape differs")
		}
	case "stream-open", "datagram-open":
		if count != 1 || f.Stream == nil || f.Stream.ID == 0 || f.Stream.ID > lanfirewall.MaxSequence || len(f.Stream.Peer) > 64 || f.Stream.Peer == "" {
			return errors.New("LAN stream request shape differs")
		}
	case "stream-ready", "datagram-ready":
		if count != 1 || f.Stream == nil || f.Stream.ID == 0 || f.Stream.ID > lanfirewall.MaxSequence || f.Stream.Peer != "" {
			return errors.New("LAN stream acknowledgement shape differs")
		}
	case "stop", "closed", "worker-ready":
		if count != 0 {
			return errors.New("LAN closure message has unexpected body")
		}
	default:
		return errors.New("unsupported fixed LAN v2 message")
	}
	return nil
}

// Walk before decoding so duplicate keys at every nesting depth, malformed
// suffixes and excessive nesting cannot be hidden by encoding/json's overwrite.
func strictShape(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	tokens := 0
	var value func(int) error
	value = func(depth int) error {
		tokens++
		if tokens > 4096 || depth > 16 {
			return errors.New("LAN JSON nesting/token budget exceeded")
		}
		token, e := d.Token()
		if e != nil {
			return errors.New("LAN JSON malformed")
		}
		delimiter, container := token.(json.Delim)
		if !container {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				name, ok := key.(string)
				if e != nil || !ok || seen[name] {
					return errors.New("LAN duplicate/malformed JSON key")
				}
				seen[name] = true
				if e = value(depth + 1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return errors.New("LAN malformed object")
			}
		case '[':
			for d.More() {
				if e := value(depth + 1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return errors.New("LAN malformed array")
			}
		default:
			return errors.New("LAN JSON delimiter invalid")
		}
		return nil
	}
	if e := value(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return errors.New("LAN JSON trailing data")
	}
	return nil
}

// encoding/json permits case-insensitive matches even with DisallowUnknownFields.
// Walk the exact declared schema first; a lone wrong-case key is invalid too.
func canonicalKeys(raw json.RawMessage, t reflect.Type) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("LAN null typed value refused")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(time.Time{}) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil || object == nil {
			return errors.New("LAN typed object shape differs")
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
		for key, value := range object {
			field, ok := fields[key]
			if !ok {
				return errors.New("LAN JSON field spelling/schema differs")
			}
			if e := canonicalKeys(value, field); e != nil {
				return e
			}
		}
	case reflect.Slice, reflect.Array:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil || items == nil {
			return errors.New("LAN typed array shape differs")
		}
		for _, item := range items {
			if e := canonicalKeys(item, t.Elem()); e != nil {
				return e
			}
		}
	}
	return nil
}

// ValidateClosedJSON reuses the bounded duplicate/null/exact-spelling parser
// for the ordinary owner-facing LAN requests. Parsing confers no native authority.
func ValidateClosedJSON(data []byte, shape any) error {
	if len(data) == 0 || len(data) > FrameBudget || shape == nil {
		return errors.New("LAN closed request exceeds bounds")
	}
	if err := strictShape(data); err != nil {
		return err
	}
	return canonicalKeys(data, reflect.TypeOf(shape))
}
func Decode(data []byte) (Frame, error) {
	var frame Frame
	if len(data) == 0 || len(data) > FrameBudget {
		return frame, errors.New("LAN v2 frame exceeds bound")
	}
	if e := strictShape(data); e != nil {
		return frame, e
	}
	if e := canonicalKeys(data, reflect.TypeOf(frame)); e != nil {
		return frame, e
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&frame) != nil {
		return Frame{}, errors.New("LAN v2 frame schema differs")
	}
	if e := frame.Validate(); e != nil {
		return Frame{}, e
	}
	return frame, nil
}
func Read(r io.Reader) (Frame, error) {
	var header [4]byte
	if _, e := io.ReadFull(r, header[:]); e != nil {
		return Frame{}, e
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > FrameBudget {
		return Frame{}, errors.New("LAN v2 frame exceeds bound")
	}
	data := make([]byte, size)
	if _, e := io.ReadFull(r, data); e != nil {
		return Frame{}, e
	}
	return Decode(data)
}
func Write(w io.Writer, frame Frame) error {
	if e := frame.Validate(); e != nil {
		return e
	}
	data, e := json.Marshal(frame)
	if e != nil || len(data) > FrameBudget {
		return errors.New("LAN v2 encoded frame exceeds bound")
	}
	var h [4]byte
	binary.LittleEndian.PutUint32(h[:], uint32(len(data)))
	for _, part := range [][]byte{h[:], data} {
		for len(part) > 0 {
			n, e := w.Write(part)
			if e != nil {
				return e
			}
			if n <= 0 || n > len(part) {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}

// Session guards one fixed administrative setup conversation. Native peer
// binding is required by the caller before accepting any stage. Unknown outcome
// never resets this state to permit a second non-idempotent stage request.
type Session struct {
	phase    string
	sequence uint64
	lease    string
}

func (s *Session) Accept(f Frame) error {
	if e := f.Validate(); e != nil {
		return e
	}
	want := "stage-request"
	switch s.phase {
	case "":
	case "stage-request":
		want = "prepared"
	case "prepared":
		want = "child-ack"
	case "child-ack":
		want = "commit"
	default:
		return errors.New("LAN setup session already terminal/unknown")
	}
	if f.Kind != want || f.Sequence != s.sequence+1 || s.lease != "" && f.LeaseSHA != s.lease {
		return errors.New("LAN setup stage/replay differs")
	}
	if f.Kind == "prepared" {
		s.lease = f.LeaseSHA
	}
	s.phase = f.Kind
	s.sequence = f.Sequence
	return nil
}
func (s *Session) Unknown() { s.phase = "unknown" }
