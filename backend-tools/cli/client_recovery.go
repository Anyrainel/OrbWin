package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sync"
	"time"
)

var errClientConfigNoRecovery = errors.New("no interrupted private Docker configuration write needs recovery")

const zeroWriterDigest = "0000000000000000000000000000000000000000000000000000000000000000"

// Version 3 is reserved for producers which durably journal the actual suspended
// root before its first ResumeThread. Version 2 cannot supply no-start evidence.
type writerIntentRecord struct {
	Schema         uint32 `json:"schema"`
	Kind           string `json:"kind"`
	Mutation       string `json:"mutation"`
	Binding        string `json:"binding"`
	KeysSHA        string `json:"keysSha256"`
	BaseSHA        string `json:"baseSha256"`
	BaseVolume     uint32 `json:"baseVolume"`
	BaseHigh       uint32 `json:"baseHigh"`
	BaseLow        uint32 `json:"baseLow"`
	BaseSD         string `json:"baseSdSha256"`
	OriginalVolume uint32 `json:"originalVolume"`
	OriginalHigh   uint32 `json:"originalHigh"`
	OriginalLow    uint32 `json:"originalLow"`
	OriginalSD     string `json:"originalSdSha256"`
	OriginalSHA    string `json:"originalSha256"`
}
type writerPrepareRecord struct {
	Schema     uint32 `json:"schema"`
	Kind       string `json:"kind"`
	Mutation   string `json:"mutation"`
	Binding    string `json:"binding"`
	KeysSHA    string `json:"keysSha256"`
	BaseSHA    string `json:"baseSha256"`
	BaseVolume uint32 `json:"baseVolume"`
	BaseHigh   uint32 `json:"baseHigh"`
	BaseLow    uint32 `json:"baseLow"`
	BaseSD     string `json:"baseSdSha256"`
}

func (v writerPrepareRecord) intent() writerIntentRecord {
	return writerIntentRecord{Schema: v.Schema, Kind: "docker-writer", Mutation: v.Mutation, Binding: v.Binding, KeysSHA: v.KeysSHA, BaseSHA: v.BaseSHA, BaseVolume: v.BaseVolume, BaseHigh: v.BaseHigh, BaseLow: v.BaseLow, BaseSD: v.BaseSD, OriginalSHA: v.BaseSHA, OriginalSD: v.BaseSD}
}
func parseWriterPrepare(body []byte) (v writerPrepareRecord, e error) {
	if closedWriterRecord(body, 4096, 10, &v) != nil || v.Schema != 3 || v.Kind != "docker-writer-prepare" {
		return v, errClientConfigRecovery
	}
	encoded, _ := json.Marshal(v.intent())
	_, e = parseWriterIntent(encoded)
	return v, e
}
func (v writerPrepareRecord) matches(i writerIntentRecord) bool {
	p := v.intent()
	return p.Schema == i.Schema && p.Mutation == i.Mutation && p.Binding == i.Binding && p.KeysSHA == i.KeysSHA && p.BaseSHA == i.BaseSHA && p.BaseVolume == i.BaseVolume && p.BaseHigh == i.BaseHigh && p.BaseLow == i.BaseLow && p.BaseSD == i.BaseSD
}

type writerBirthRecord struct {
	Schema      uint32 `json:"schema"`
	Kind        string `json:"kind"`
	Binding     string `json:"binding"`
	IntentSHA   string `json:"intentSha256"`
	IntentBody  string `json:"intentBody"`
	RootPID     uint32 `json:"rootPid"`
	RootCreated uint64 `json:"rootCreated"`
	RootSession uint32 `json:"rootSession"`
}
type writerRecoveryRecord struct {
	Schema        uint32 `json:"schema"`
	Kind          string `json:"kind"`
	Binding       string `json:"binding"`
	CurrentVolume uint32 `json:"currentVolume"`
	CurrentHigh   uint32 `json:"currentHigh"`
	CurrentLow    uint32 `json:"currentLow"`
	CurrentSD     string `json:"currentSdSha256"`
	CurrentSHA    string `json:"currentSha256"`
	RestoreSHA    string `json:"restoreSha256"`
	IntentBody    string `json:"intentBody"`
}

func lowerDigest(value string) bool {
	decoded, e := hex.DecodeString(value)
	return e == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}
func digestBytes(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func closedWriterRecord(body []byte, budget, fields int, target any) error {
	if len(body) == 0 || len(body) > budget {
		return errClientConfigRecovery
	}
	parsed, e := checkedClientJSON(body)
	if e != nil {
		return errClientConfigRecovery
	}
	obj, ok := parsed.(map[string]any)
	if !ok || len(obj) != fields {
		return errClientConfigRecovery
	}
	// encoding/json accepts case aliases and null scalar fields. These records
	// are a closed cross-language schema, so require exact names and types first.
	typ := reflect.TypeOf(target).Elem()
	for j := 0; j < typ.NumField(); j++ {
		field := typ.Field(j)
		value, present := obj[field.Tag.Get("json")]
		if !present {
			return errClientConfigRecovery
		}
		if field.Type.Kind() == reflect.String {
			if _, ok := value.(string); !ok {
				return errClientConfigRecovery
			}
		} else if field.Type.Kind() == reflect.Bool {
			if _, ok := value.(bool); !ok {
				return errClientConfigRecovery
			}
		} else if _, ok := value.(json.Number); !ok {
			return errClientConfigRecovery
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return errClientConfigRecovery
	}
	return nil
}
func parseWriterIntent(body []byte) (v writerIntentRecord, e error) {
	if closedWriterRecord(body, 4096, 15, &v) != nil || v.Schema != 3 || v.Kind != "docker-writer" || (v.Mutation != string(writerRegistryAuth) && v.Mutation != string(writerRootUnchanged)) {
		return v, errClientConfigRecovery
	}
	for _, digest := range []string{v.Binding, v.KeysSHA, v.BaseSHA, v.BaseSD, v.OriginalSD, v.OriginalSHA} {
		if !lowerDigest(digest) {
			return v, errClientConfigRecovery
		}
	}
	if v.BaseSHA != v.OriginalSHA {
		return v, errClientConfigRecovery
	}
	if v.Binding == zeroWriterDigest || v.KeysSHA == zeroWriterDigest {
		return v, errClientConfigRecovery
	}
	return v, nil
}
func parseWriterBirth(body []byte) (b writerBirthRecord, intent writerIntentRecord, e error) {
	if closedWriterRecord(body, 8192, 8, &b) != nil || b.Schema != 3 || b.Kind != "docker-writer-birth" || b.RootPID == 0 || b.RootCreated == 0 || !lowerDigest(b.Binding) || !lowerDigest(b.IntentSHA) || digestBytes([]byte(b.IntentBody)) != b.IntentSHA {
		return b, intent, errClientConfigRecovery
	}
	intent, e = parseWriterIntent([]byte(b.IntentBody))
	if e != nil || intent.Binding != b.Binding {
		return b, intent, errClientConfigRecovery
	}
	return b, intent, nil
}

// An EOF prefix is the only incomplete-birth case. A complete JSON document,
// invalid token, wrong schema or mismatched digest remains corrupt evidence.
func incompleteWriterBirth(body []byte) bool {
	if len(body) > 8192 {
		return false
	}
	// Every producer writes a JSON object. EOF inside an array or string is
	// corrupt evidence, never a prefix of one of our interrupted journals.
	prefix := bytes.TrimLeft(body, " \t\r\n")
	if len(prefix) != 0 && prefix[0] != '{' {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	var value any
	err := decoder.Decode(&value)
	return err == io.EOF || err == io.ErrUnexpectedEOF
}
func parseWriterRecovery(body []byte, binding, restoreSHA string) (v writerRecoveryRecord, e error) {
	if closedWriterRecord(body, 8192, 10, &v) != nil || v.Schema != 1 || v.Kind != "restore-original" || v.Binding != binding || v.RestoreSHA != restoreSHA || !lowerDigest(v.CurrentSD) || !lowerDigest(v.CurrentSHA) || !lowerDigest(v.RestoreSHA) {
		return v, errClientConfigRecovery
	}
	intent, err := parseWriterIntent([]byte(v.IntentBody))
	if err != nil || intent.Binding != binding {
		return v, errClientConfigRecovery
	}
	return v, nil
}

// A durable birth can still be followed by cancellation. Only this ordering
// crosses into user code; any failed journal or cancellation leaves no resume.
func writerBeforeFirstResume(ctx context.Context, birth func() error, resume func() error) error {
	if ctx == nil || ctx.Err() != nil || birth == nil || resume == nil {
		return errClientConfigRecovery
	}
	if e := birth(); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	return resume()
}

// The recorded generation is compared with a HANDLE observation, never a PID
// existence guess. The native caller must positively close that query handle.
func writerGenerationStopped(expected, actual writerBirthRecord, signaled bool, queryErr error) bool {
	return queryErr == nil && actual.RootPID == expected.RootPID && actual.RootCreated != 0 && (actual.RootCreated != expected.RootCreated || (actual.RootSession == expected.RootSession && signaled))
}

// The review is a process-local, one-use capability. It is never marshaled or
// reconstructed from CLI arguments. Native callbacks own controller + all exact
// records and revalidate them before publishing. No authentication is replayed.
type clientWriterRecovery struct {
	sync.Mutex
	deadline            time.Time
	used                bool
	finishOnly          bool
	preparingOnly       bool
	localReplace        bool
	freshInitialization bool
	restoreNative       func() (*clientConfigGuard, error)
	closeNative         func() error
}

func (r *clientWriterRecovery) summary() string {
	if r != nil && r.freshInitialization {
		return "Finish initializing private Docker settings using empty credential settings and the current managed selector. No credential command was started. Existing user settings or unknown files will not be replaced."
	}
	if r != nil && r.localReplace {
		return "Keep the current validated private Docker settings and discard the interrupted unpublished settings stage. Finish the local settings change, then refresh managed tool routing. No saved original is restored and no credential command is replayed."
	}
	if r != nil && r.preparingOnly {
		return "Finish interrupted private settings preparation while preserving the unchanged source settings. No credential command was resumed. Any unpublished recovery stage will be discarded."
	}
	if r != nil && r.finishOnly {
		return "An interrupted private configuration cleanup can be completed. The recorded writer is stopped or never resumed. No authentication, credential-vault operation, or registry request will be replayed."
	}
	return "Restore the exact durable private Docker configuration baseline and refresh its current managed selector. The recorded writer is stopped or never resumed. Credentials or registry changes may already have occurred; they will not be replayed or undone. Uncertain child output will be discarded."
}
func (r *clientWriterRecovery) acceptsRestoreOriginal() bool {
	return r != nil && !r.localReplace && !r.freshInitialization
}
func (r *clientWriterRecovery) close() error {
	if r == nil {
		return nil
	}
	r.Lock()
	defer r.Unlock()
	if r.closeNative == nil {
		return nil
	}
	close := r.closeNative
	r.closeNative = nil
	r.used = true
	return close()
}
func (r *clientWriterRecovery) restore(ctx context.Context) (*clientConfigGuard, error) {
	if r == nil {
		return nil, errClientConfigRecovery
	}
	r.Lock()
	defer r.Unlock()
	if ctx == nil || ctx.Err() != nil || r.used || r.restoreNative == nil || time.Now().After(r.deadline) {
		return nil, errClientConfigRecovery
	}
	r.used = true
	guard, e := r.restoreNative()
	if e == nil {
		r.closeNative = nil
	}
	return guard, e
}
