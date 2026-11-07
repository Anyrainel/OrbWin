//go:build windows && amd64

package lanprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unsafe"
)

const fixtureReceiptSDDL = "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)"

type fixtureProtectedRoot struct {
	path       string
	handle     windows.Handle
	descriptor *windows.SECURITY_DESCRIPTOR
}
type fixtureDiskJournal struct {
	mu                      sync.Mutex
	file                    *os.File
	actor                   string
	writes, bytes           int
	policy                  fixtureNativePolicy
	programSHA, snapshotSHA string
}

func fixtureHandleMetadata(h windows.Handle, directory bool) error {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return errors.New("fixture receipt/file handle type or reparse refusal")
	}
	return nil
}
func fixtureCheckReceiptSecurity(h windows.Handle, want *windows.SECURITY_DESCRIPTOR) error {
	sd, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil || sd.String() != want.String() {
		return errors.New("fixture protected receipt descriptor differs")
	}
	return nil
}
func fixtureNewProtectedRoot(nonce string) (*fixtureProtectedRoot, error) {
	if len(nonce) != 32 || !fixtureNamePattern.MatchString("OrbWin-LAN-proof-"+nonce+"-i") {
		return nil, errors.New("fixture receipt nonce invalid")
	}
	base, e := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if e != nil || !filepath.IsAbs(base) || strings.HasPrefix(base, `\\`) {
		return nil, errors.New("fixture local ProgramData unavailable")
	}
	// KnownFolder location and every existing ancestor must be local directories,
	// not junctions/symlinks. This is checked observation, not hostile-admin CAS.
	for current := base; ; current = filepath.Dir(current) {
		p, _ := windows.UTF16PtrFromString(current)
		h, e := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if e != nil {
			return nil, errors.New("fixture receipt ancestor unavailable")
		}
		e = fixtureHandleMetadata(h, true)
		windows.CloseHandle(h)
		if e != nil {
			return nil, e
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	sd, e := windows.SecurityDescriptorFromString(fixtureReceiptSDDL)
	if e != nil {
		return nil, errors.New("fixture receipt security unavailable")
	}
	dir := filepath.Join(base, "OrbWin-LAN-proof-"+nonce)
	p, _ := windows.UTF16PtrFromString(dir)
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if windows.CreateDirectory(p, &sa) != nil {
		return nil, errors.New("fixture fresh protected receipt directory refused")
	}
	h, e := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, errors.New("fixture protected directory open refused")
	}
	if e = errors.Join(fixtureHandleMetadata(h, true), fixtureCheckReceiptSecurity(h, sd)); e != nil {
		windows.CloseHandle(h)
		return nil, e
	}
	return &fixtureProtectedRoot{path: dir, handle: h, descriptor: sd}, nil
}
func (r *fixtureProtectedRoot) close() error { return windows.CloseHandle(r.handle) }
func (r *fixtureProtectedRoot) newJournal(actor string) (*fixtureDiskJournal, error) {
	if actor != "A" && actor != "B" && actor != "C" && actor != "D" && actor != "U" {
		return nil, errors.New("fixture actor invalid")
	}
	p, _ := windows.UTF16PtrFromString(filepath.Join(r.path, actor+".jsonl"))
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: r.descriptor}
	h, e := windows.CreateFile(p, windows.GENERIC_WRITE|windows.READ_CONTROL, windows.FILE_SHARE_READ, &sa, windows.CREATE_NEW, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_WRITE_THROUGH, 0)
	if e != nil {
		return nil, errors.New("fixture fresh protected journal unavailable")
	}
	if e = errors.Join(fixtureHandleMetadata(h, false), fixtureCheckReceiptSecurity(h, r.descriptor)); e != nil {
		windows.CloseHandle(h)
		return nil, e
	}
	return &fixtureDiskJournal{file: os.NewFile(uintptr(h), actor+".jsonl"), actor: actor}, nil
}
func (j *fixtureDiskJournal) record(stage string, i fixtureIntent, c fixtureCompletion) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if i.actor != j.actor || i != j.policy.intent || !fixtureNamePattern.MatchString(i.name) || j.writes >= 16 || len(j.programSHA) != 64 {
		return errors.New("fixture journal identity/budget refusal")
	}
	switch stage {
	case "create-intent", "create-result", "typed-snapshot", "remove-intent", "remove-result":
	default:
		return errors.New("fixture journal stage refusal")
	}
	row := struct {
		Version                                                         int `json:"version"`
		Stage, Name, Actor                                              string
		Dispatched, Terminal, Closed                                    bool
		Status                                                          uint32
		ReturnedFullKeys                                                bool
		OwnedKeys                                                       []string `json:"ownedKeys,omitempty"`
		Program, ProgramSHA, LoopbackAlias, LocalPort, TypedSnapshotSHA string
		Disabled, Block                                                 uint16
	}{Version: 1, Stage: stage, Name: i.name, Actor: i.actor, Dispatched: c.dispatched, Terminal: c.terminal, Closed: c.closed, Status: c.status, ReturnedFullKeys: c.returnedFullKeys}
	row.Program = j.policy.program
	row.ProgramSHA = j.programSHA
	row.LoopbackAlias = j.policy.alias
	row.LocalPort = j.policy.port
	row.Disabled = fixtureDisabled
	row.Block = fixtureBlock
	row.TypedSnapshotSHA = j.snapshotSHA
	// Only this actor's positively acknowledged key set may be persisted. Unknown
	// or duplicate results never cause foreign instances/keys to enter receipts.
	if stage == "create-result" {
		if fixturePositiveCompletion(i, c) {
			row.OwnedKeys = []string{c.keys.instanceID, c.keys.creationClassName, c.keys.policyRuleName, c.keys.systemCreationClassName, c.keys.systemName}
		}
	}
	if stage == "typed-snapshot" || stage == "remove-intent" || stage == "remove-result" {
		if !fixtureCompleteKeys(c.keys) || c.keys.instanceID != i.name {
			return errors.New("fixture retirement receipt keys differ")
		}
		row.OwnedKeys = []string{c.keys.instanceID, c.keys.creationClassName, c.keys.policyRuleName, c.keys.systemCreationClassName, c.keys.systemName}
	}
	data, e := json.Marshal(row)
	if e != nil || len(data) > 16384 || j.bytes+len(data)+1 > 65536 {
		return errors.New("fixture journal payload budget")
	}
	data = append(data, '\n')
	n, e := j.file.Write(data)
	if e != nil || n != len(data) {
		return errors.New("fixture journal append failed; dispatch/authority withheld")
	}
	if j.file.Sync() != nil {
		return errors.New("fixture journal flush failed; dispatch/authority withheld")
	}
	j.writes++
	j.bytes += len(data)
	return nil
}
func (j *fixtureDiskJournal) recordDefinition(owner fixtureOwnership, d fixtureDefinition) error {
	if owner.keys != d.keys || owner.intent != j.policy.intent {
		return errors.New("fixture snapshot ownership differs")
	}
	type property struct {
		Name                string
		Kind, Flags, Scalar uint32
		Text                string
		Shorts              []uint16
		Texts               []string
		Timestamp           bool
		Parts               [7]uint32
		UTC                 int32
	}
	type instance struct {
		Class      string
		Properties []property
	}
	instances := []typedInstance{d.rule}
	instances = append(instances, d.filters[:]...)
	out := []instance{}
	for _, v := range instances {
		names := []string{}
		for name := range v.values {
			names = append(names, name)
		}
		sort.Strings(names)
		row := instance{Class: v.class}
		for _, name := range names {
			p := v.values[name]
			row.Properties = append(row.Properties, property{name, p.kind, p.flags, p.scalar, p.text, p.shorts, p.texts, p.date.timestamp, p.date.parts, p.date.utc})
		}
		out = append(out, row)
	}
	data, e := json.Marshal(out)
	if e != nil || len(data) > 256<<10 {
		return errors.New("fixture snapshot hash input bound")
	}
	sum := sha256.Sum256(data)
	j.snapshotSHA = hex.EncodeToString(sum[:])
	return j.record("typed-snapshot", owner.intent, fixtureCompletion{keys: owner.keys, returnedFullKeys: true})
}
func fixtureHoldOwnProgram() (file *os.File, program, sha string, err error) {
	program, err = os.Executable()
	if err != nil || !filepath.IsAbs(program) || strings.ContainsAny(program, "%*?[]`\x00") || strings.HasPrefix(program, `\\`) {
		return nil, "", "", errors.New("fixture own local executable path unavailable")
	}
	p, _ := windows.UTF16PtrFromString(program)
	h, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, "", "", errors.New("fixture executable exclusive read handle unavailable")
	}
	if e = fixtureHandleMetadata(h, false); e != nil {
		windows.CloseHandle(h)
		return nil, "", "", e
	}
	file = os.NewFile(uintptr(h), program)
	info, e := file.Stat()
	if e != nil || info.Size() <= 0 || info.Size() > 32<<20 {
		file.Close()
		return nil, "", "", errors.New("fixture executable size bound")
	}
	sum := sha256.New()
	n, e := io.Copy(sum, io.LimitReader(file, 32<<20+1))
	if e != nil || n != info.Size() {
		file.Close()
		return nil, "", "", errors.New("fixture executable bounded hash failed")
	}
	return file, program, hex.EncodeToString(sum.Sum(nil)), nil
}
