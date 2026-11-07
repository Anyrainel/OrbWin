//go:build windows

package lanhelper

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
)

// RecoveredReview is minted only by an exact native protected-file read. It is
// correlation/denial information, never permission to restart/adopt a listener.
// Missing records and a busy writer remain unknown, not absence proof.
type RecoveredReview struct {
	record   OwnerReview
	prepared *Prepared
}

func ReadProtectedReviewObservation(ctx context.Context, installation, reviewID, reviewSHA string) (ProtectedReviewObservation, error) {
	if ctx == nil || ctx.Err() != nil {
		return ProtectedReviewObservation{}, errors.New("bounded protected review context required")
	}
	recovered, err := ReadProtectedReview(installation, reviewID, reviewSHA)
	if err != nil || ctx.Err() != nil {
		return ProtectedReviewObservation{}, errors.New("protected original review unavailable")
	}
	return ProtectedReviewObservation{Record: recovered.record, Prepared: recovered.prepared}, nil
}

func (recovered *RecoveredReview) Correlation() (runtime, review, sha, operation string) {
	if recovered == nil {
		return "", "", "", ""
	}
	record := recovered.record
	return record.RuntimeInstance, record.ReviewID, record.ReviewSHA256, record.OperationID
}

// ReadProtectedReview performs no elevation, directory creation, ACL repair,
// mutation or engine access. The owner must still bound and join this native
// observation in its read-only child lifetime before exposing its result.
func ReadProtectedReview(installation, reviewID, reviewSHA string) (result *RecoveredReview, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var threadToken windows.Token
	if e := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &threadToken); e == nil {
		threadToken.Close()
		return nil, errors.New("impersonated protected review read refused")
	} else if !errors.Is(e, windows.ERROR_NO_TOKEN) {
		return nil, errors.New("protected review native thread identity unavailable")
	}
	if !recordID(installation) || !recordID(reviewID) || !ValidSHA(reviewSHA) {
		return nil, errors.New("fixed protected review identity required")
	}
	sid, _, err := nativeToken(windows.CurrentProcess(), false)
	if err != nil {
		return nil, err
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil || len(base) < 3 || base[1:3] != `:\` || strings.ContainsAny(base, `/%*?[]`+"`\x00") {
		return nil, errors.New("fixed local protected history root required")
	}
	var held []*os.File
	defer func() {
		for index := len(held) - 1; index >= 0; index-- {
			err = errors.Join(err, held[index].Close())
		}
		if err != nil {
			result = nil
		}
	}()
	for current := base; ; current = filepath.Dir(current) {
		file, e := heldFile(current, true, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
		if e != nil {
			return nil, e
		}
		held = append(held, file)
		if filepath.Dir(current) == current {
			break
		}
	}
	directorySD, err := windows.SecurityDescriptorFromString(protectedSDDL(sid, true))
	if err != nil {
		return nil, err
	}
	root := filepath.Join(base, "OrbWin", "LANRecords", installation)
	for _, path := range []string{filepath.Join(base, "OrbWin"), filepath.Join(base, "OrbWin", "LANRecords"), root} {
		file, e := heldFile(path, true, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
		if e != nil {
			return nil, e
		}
		held = append(held, file)
		if e = exactDescriptor(file, directorySD); e != nil {
			return nil, e
		}
	}
	file, err := heldFile(filepath.Join(root, reviewID+".jsonl"), false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return nil, err
	}
	held = append(held, file)
	fileSD, err := windows.SecurityDescriptorFromString(protectedSDDL(sid, false))
	if err != nil {
		return nil, err
	}
	if err = exactDescriptor(file, fileSD); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, protectedJournalBudget+1))
	if err != nil {
		return nil, err
	}
	history, err := ParseProtectedHistory(data)
	if err != nil {
		return nil, err
	}
	record := history.Review
	if record.ReviewID != reviewID || record.ReviewSHA256 != reviewSHA || record.Stage.Base.Installation != installation || record.Stage.Base.UserSID != sid {
		return nil, errors.New("protected original review correlation differs")
	}
	return &RecoveredReview{record: record, prepared: history.Prepared}, nil
}
