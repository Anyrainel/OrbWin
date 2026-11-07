//go:build windows && amd64

package lanprovider

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanlease"
)

type ownedProtectedHandle struct {
	file         *os.File
	identity     windows.ByHandleFileInformation
	descriptor   *windows.SECURITY_DESCRIPTOR
	closeUnknown bool
}

func protectedRuleSD(sid string, directory bool) string {
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	return "O:BAD:P(A;" + inherit + ";FA;;;SY)(A;" + inherit + ";FA;;;BA)(A;" + inherit + ";FRFX;;;" + sid + ")"
}
func ownedOrdinarySID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", errors.New("bounded protected LAN read context required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	thread, err := windows.GetCurrentThread()
	if err != nil {
		return "", errors.New("protected LAN native thread unavailable")
	}
	var token windows.Token
	if err = windows.OpenThreadToken(thread, windows.TOKEN_QUERY, true, &token); err == nil {
		_ = token.Close()
		return "", errors.New("ordinary unimpersonated SDK process required")
	}
	if err != windows.ERROR_NO_TOKEN {
		return "", errors.New("protected LAN effective token unknown")
	}
	var elevation, size uint32
	processToken := windows.GetCurrentProcessToken()
	if err = windows.GetTokenInformation(processToken, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), 4, &size); err != nil || size != 4 || elevation != 0 {
		return "", errors.New("ordinary SDK token required")
	}
	var session, actualSession uint32
	if windows.GetTokenInformation(processToken, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &size) != nil || size != 4 || session == 0 || windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &actualSession) != nil || actualSession != session {
		return "", errors.New("ordinary SDK native session required")
	}
	user, err := processToken.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return "", errors.New("protected LAN native user unavailable")
	}
	sid := user.User.Sid.String()
	// Pure request validation later establishes ordinary account SID shape too.
	if !(strings.HasPrefix(sid, "S-1-5-21-") || strings.HasPrefix(sid, "S-1-12-1-")) {
		return "", errors.New("ordinary SDK account SID required")
	}
	return sid, nil
}
func ownedProtectedOpen(path string, directory bool, sd *windows.SECURITY_DESCRIPTOR) (out ownedProtectedHandle, err error) {
	return ownedProtectedOpenMode(path, directory, sd, false)
}
func ownedProtectedOpenMode(path string, directory bool, sd *windows.SECURITY_DESCRIPTOR, write bool) (out ownedProtectedHandle, err error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return out, errors.New("fixed protected LAN path invalid")
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	access := uint32(windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	} else {
		access |= windows.GENERIC_READ
	}
	if write {
		if directory {
			return out, ErrCleanupPending
		}
		access |= windows.GENERIC_WRITE
	}
	share := uint32(windows.FILE_SHARE_READ)
	if directory {
		share |= windows.FILE_SHARE_WRITE
	} // Directory leases deny rename/delete, not sibling staging.
	h, e := windows.CreateFile(p, access, share, nil, windows.OPEN_EXISTING, flags, 0)
	if e != nil {
		return out, errors.New("protected LAN file busy, missing or unavailable")
	}
	file := os.NewFile(uintptr(h), "protected-native-LAN-record")
	if file == nil {
		_ = windows.CloseHandle(h)
		return out, errors.New("protected LAN held file unavailable")
	}
	out.file = file
	out.descriptor = sd
	defer func() {
		if err != nil {
			if file.Close() != nil {
				out.closeUnknown = true
				err = ErrCleanupPending
			} else {
				out = ownedProtectedHandle{}
			}
		}
	}()
	if kind, e := windows.GetFileType(h); e != nil || kind != windows.FILE_TYPE_DISK {
		return out, errors.New("protected LAN local disk object required")
	}
	var final [32768]uint16
	n, e := windows.GetFinalPathNameByHandle(h, &final[0], uint32(len(final)), 0)
	if e != nil || n == 0 || n >= uint32(len(final)) {
		return out, errors.New("protected LAN native final path unavailable")
	}
	actual := strings.TrimPrefix(windows.UTF16ToString(final[:n]), `\\?\`)
	if !strings.EqualFold(actual, path) {
		return out, errors.New("protected LAN final path differs")
	}
	if e = windows.GetFileInformationByHandle(h, &out.identity); e != nil || out.identity.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (out.identity.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return out, errors.New("protected LAN path kind/reparse refusal")
	}
	if !directory && (out.identity.NumberOfLinks != 1 || out.identity.FileSizeHigh != 0) {
		return out, errors.New("protected LAN regular single-link file required")
	}
	if e = out.recheck(); e != nil {
		return out, e
	}
	return out, nil
}
func (h ownedProtectedHandle) recheck() error {
	var now windows.ByHandleFileInformation
	if h.file == nil || windows.GetFileInformationByHandle(windows.Handle(h.file.Fd()), &now) != nil {
		return errors.New("protected LAN held file metadata unavailable")
	}
	expected := h.identity
	if expected.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		if now.FileAttributes != expected.FileAttributes || now.VolumeSerialNumber != expected.VolumeSerialNumber || now.FileIndexHigh != expected.FileIndexHigh || now.FileIndexLow != expected.FileIndexLow || now.CreationTime != expected.CreationTime {
			return errors.New("protected LAN held directory identity changed")
		}
	} else {
		// Our read may update last-access bookkeeping; it cannot change data/write
		// time, identity, link count, size or attributes while the read lease is held.
		now.LastAccessTime = windows.Filetime{}
		expected.LastAccessTime = windows.Filetime{}
		if now != expected {
			return errors.New("protected LAN held file metadata changed")
		}
	}
	if h.descriptor != nil {
		got, err := windows.GetSecurityInfo(windows.Handle(h.file.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil || got == nil || got.String() != h.descriptor.String() {
			return errors.New("protected LAN exact native descriptor differs")
		}
	}
	return nil
}
func protectedRuleRead(h ownedProtectedHandle, budget int) ([]byte, error) {
	if h.identity.FileSizeLow == 0 || uint64(h.identity.FileSizeLow) > uint64(budget) {
		return nil, errors.New("protected LAN native file exceeds budget")
	}
	data, err := io.ReadAll(io.LimitReader(h.file, int64(budget+1)))
	if err != nil || len(data) != int(h.identity.FileSizeLow) {
		return nil, errors.New("protected LAN read incomplete")
	}
	if err = h.recheck(); err != nil {
		return nil, err
	}
	return data, nil
}

// ReadProtectedRule produces observation-only ACK metadata. It cannot construct
// OwnedRule, grant ingress, restart a worker, dispatch mutation or restore a
// prior manager. An external read-only child owner must bound/join synchronous
// filesystem/MI calls; cancellation alone is not native retirement evidence.
func ReadProtectedRule(ctx context.Context, installation, leaseSHA string) (ack lanlease.RuleAcknowledgement, err error) {
	err = readProtectedObservation(ctx, installation, leaseSHA, false, func(ctx context.Context, r ownedProtectedObservation, d traversalDeclarations) error {
		current, e := ownedReadDefinition(ctx, r.definition.keys, d)
		if e != nil {
			return ErrCleanupPending
		}
		ack, e = acknowledgeProtectedRule(r, current, time.Now().UTC())
		return e
	})
	if err != nil {
		ack = lanlease.RuleAcknowledgement{}
	}
	return
}

// ReadProtectedRemoval is positive removed5 +fresh exact absence only. It never
// dispatches Delete or grants ingress; cross-owner closure uses ReadProtectedCleanup.
func ReadProtectedRemoval(ctx context.Context, installation, leaseSHA string) (ack lanlease.RemovalAcknowledgement, err error) {
	err = readProtectedObservation(ctx, installation, leaseSHA, true, func(ctx context.Context, r ownedProtectedObservation, d traversalDeclarations) error {
		if r.journalPhase != "removed" {
			return ErrCleanupPending
		}
		absent, e := ownedNameAbsent(ctx, r.policy.receipt.RuleName, d.rule)
		if e != nil {
			return ErrCleanupPending
		}
		ack, e = acknowledgeProtectedRemoval(r, absent)
		return e
	})
	if err != nil {
		ack = lanlease.RemovalAcknowledgement{}
	}
	return
}

// ReadProtectedCleanup is read-only native prior-owner evidence. The immutable
// original mapping is never rewritten to the current SDK generation. It cannot
// mint a rule owner, deletion capability, Ready or replay authority.
func ReadProtectedCleanup(ctx context.Context, installation, leaseSHA string) (out CleanupObservation, err error) {
	err = readProtectedObservation(ctx, installation, leaseSHA, true, func(ctx context.Context, r ownedProtectedObservation, d traversalDeclarations) error {
		if cleanupChildExited(ctx, r.metadata) != nil || observeCleanupNamespace(ctx, r.metadata) != nil {
			return ErrCleanupPending
		}
		absent := false
		if r.journalPhase == "created" {
			current, e := ownedReadDefinition(ctx, r.definition.keys, d)
			if e != nil {
				return ErrCleanupPending
			}
			if _, e = acknowledgeProtectedRule(r, current, time.Now().UTC()); e != nil {
				return ErrCleanupPending
			}
		} else {
			var e error
			absent, e = ownedNameAbsent(ctx, r.policy.receipt.RuleName, d.rule)
			if e != nil || !absent {
				return ErrCleanupPending
			}
		}
		if cleanupChildExited(ctx, r.metadata) != nil || observeCleanupNamespace(ctx, r.metadata) != nil {
			return ErrCleanupPending
		}
		out = cleanupObservation(r, absent, time.Now().UTC())
		return nil
	})
	if err != nil {
		out = CleanupObservation{}
	}
	return
}
func readProtectedObservation(ctx context.Context, installation, leaseSHA string, cleanup bool, observe func(context.Context, ownedProtectedObservation, traversalDeclarations) error) (err error) {
	if ctx == nil || !ownedHex(installation, 16) || !ownedHex(leaseSHA, 32) {
		return ErrCleanupPending
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrCleanupPending
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	sid, err := ownedOrdinarySID(ctx)
	if err != nil {
		return err
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil || len(base) < 3 || base[0] < 'A' || base[0] > 'Z' || base[1:3] != `:\` || strings.ContainsAny(base, `/%*?[]`+"`\x00") || filepath.Clean(base) != base {
		return ErrCleanupPending
	}
	var held []ownedProtectedHandle
	defer func() {
		failed := false
		for i := len(held) - 1; i >= 0; i-- {
			if held[i].closeUnknown {
				failed = true
				continue
			} // Unknown Close is not retried or treated as retirement.
			if held[i].recheck() != nil {
				failed = true
			}
			if held[i].file.Close() != nil {
				failed = true
			}
		}
		if failed || ctx.Err() != nil {
			err = ErrCleanupPending
		}
	}()
	keep := func(path string, directory bool, sd *windows.SECURITY_DESCRIPTOR) (ownedProtectedHandle, error) {
		h, e := ownedProtectedOpen(path, directory, sd)
		if h.file != nil {
			held = append(held, h)
		}
		return h, e
	}
	// Retain every native ancestor before resolving the child, rejecting reparses.
	chain := []string{base}
	for current := filepath.Dir(base); ; current = filepath.Dir(current) {
		chain = append(chain, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if _, e := keep(chain[i], true, nil); e != nil {
			return e
		}
	}
	dirSD, e := windows.SecurityDescriptorFromString(protectedRuleSD(sid, true))
	if e != nil {
		return ErrCleanupPending
	}
	root := filepath.Join(base, "OrbWin", "LANRules", installation)
	for _, path := range []string{filepath.Join(base, "OrbWin"), filepath.Join(base, "OrbWin", "LANRules"), root} {
		if _, e = keep(path, true, dirSD); e != nil {
			return e
		}
	}
	fileSD, e := windows.SecurityDescriptorFromString(protectedRuleSD(sid, false))
	if e != nil {
		return ErrCleanupPending
	}
	meta, e := keep(filepath.Join(root, leaseSHA+".owner.json"), false, fileSD)
	if e != nil {
		return e
	}
	journal, e := keep(filepath.Join(root, leaseSHA+".jsonl"), false, fileSD)
	if e != nil {
		return e
	}
	metadata, e := protectedRuleRead(meta, ownedMetadataBudget)
	if e != nil {
		return e
	}
	events, e := protectedRuleRead(journal, ownedProtectedRecordBudget)
	if e != nil {
		return e
	}
	// Strict protected metadata/event shape precedes any MI read. Fresh native
	// GetClass independently checks declarations before full-key/absence reads.
	observed, e := parseOwnedProtectedMode(metadata, events, installation, leaseSHA, sid, traversalDeclarations{}, cleanup)
	if e != nil {
		return e
	}
	if currentSID, e := ownedOrdinarySID(ctx); e != nil || currentSID != sid {
		return ErrCleanupPending
	}
	for _, h := range held {
		if h.recheck() != nil {
			return ErrCleanupPending
		}
	}
	declarations, e := readTraversalDeclarations(ctx)
	if e != nil {
		return ErrCleanupPending
	}
	observed, e = parseOwnedProtectedMode(metadata, events, installation, leaseSHA, sid, declarations, cleanup)
	if e != nil {
		return e
	}
	for _, h := range held {
		if h.recheck() != nil {
			return ErrCleanupPending
		}
	}
	err = observe(ctx, observed, declarations)
	if currentSID, e := ownedOrdinarySID(ctx); e != nil || currentSID != sid {
		return ErrCleanupPending
	}
	return err
}
