package lanhelper

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

func protectedRecordFixture(t *testing.T) (OwnerReview, Prepared) {
	t.Helper()
	now := time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC)
	identity := lanfirewall.CallerIdentity{PID: 100, UserSID: "S-1-5-21-1-2-3-1001", CreationTime: "100", ImageSHA256: strings.Repeat("b", 64)}
	request := lanfirewall.Request{Version: 1, Operation: "create", Installation: strings.Repeat("a", 32), UserSID: identity.UserSID, CallerPID: identity.PID, CallerCreationTime: identity.CreationTime, CallerImageSHA256: identity.ImageSHA256, Transaction: strings.Repeat("b", 32), Nonce: strings.Repeat("c", 64), ReviewSHA256: strings.Repeat("9", 64), IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Scope: lanfirewall.Scope{Protocol: "tcp", LocalPort: 58080, LocalAddress: "192.168.4.10", InterfaceGUID: "{11111111-1111-1111-1111-111111111111}", InterfaceLUID: "42", Profile: "private", ClientNetworks: []string{"192.168.4.0/24"}, WorkerSHA256: strings.Repeat("e", 64)}}
	publication := lanlease.Publication{OwnerInstance: strings.Repeat("7", 32), Daemon: "owned", ContainerID: strings.Repeat("a", 64), StartedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Generation: 1, HostPort: 58080, ContainerPort: 8080, Protocol: "tcp"}
	record := OwnerReview{RuntimeInstance: strings.Repeat("1", 32), ReviewID: strings.Repeat("2", 32), ReviewSHA256: request.ReviewSHA256, OperationID: strings.Repeat("3", 32), Transaction: request.Transaction, Purpose: "activate", Stage: StageRequest{Base: request, Publication: publication}}
	if err := record.Validate(identity, now); err != nil {
		t.Fatal(err)
	}
	spec := lanlease.Spec{Request: request, Publication: publication, LeaseID: strings.Repeat("d", 32), WorkerRoot: `C:\ProgramData\OrbWin\LAN`}
	spec.Worker = lanfirewall.WorkerBinding{Path: spec.WorkerRoot + `\` + request.Installation + `\` + spec.LeaseID + `\orbwin-session.exe`, FileID: strings.Repeat("f", 32), VolumeSerial: "17", SHA256: request.Scope.WorkerSHA256, InterfaceAlias: "Ethernet"}
	lease, err := lanlease.NewOwner(request.Installation, identity).Prepare(spec, now)
	if err != nil {
		t.Fatal(err)
	}
	return record, Prepared{Spec: spec, LeaseSHA: lease.Snapshot(now).LeaseSHA, Sequence: 2}
}
func TestLANProtectedJournalCorrelationChainAndTornWrites(t *testing.T) {
	record, prepared := protectedRecordFixture(t)
	var buffer bytes.Buffer
	var state protectedJournalState
	flushes := 0
	flush := func() error { flushes++; return nil }
	if err := state.append(protectedEntry{Kind: "owner-review", Review: &record}, buffer.Write, flush); err != nil {
		t.Fatal(err)
	}
	intent := stagingIntent{Tombstone: lanlease.Tombstone{LeaseID: prepared.Spec.LeaseID, WorkerPath: prepared.Spec.Worker.Path, Transaction: record.Transaction, Nonce: record.Stage.Base.Nonce}}
	if err := state.append(protectedEntry{Kind: "stage-intent", StageIntent: &intent}, buffer.Write, flush); err != nil {
		t.Fatal(err)
	}
	if err := state.append(protectedEntry{Kind: "prepared", Prepared: &prepared}, buffer.Write, flush); err != nil {
		t.Fatal(err)
	}
	history, err := ParseProtectedHistory(buffer.Bytes())
	if err != nil || history.Review.OperationID != record.OperationID || history.Prepared == nil || history.Prepared.LeaseSHA != prepared.LeaseSHA || flushes != 3 {
		t.Fatal("exact persisted history differs", err)
	}
	for _, data := range [][]byte{buffer.Bytes()[:buffer.Len()-1], append(append([]byte(nil), buffer.Bytes()...), []byte("{}\n")...), bytes.Replace(buffer.Bytes(), []byte(record.OperationID), []byte(strings.Repeat("4", 32)), 1), bytes.Replace(buffer.Bytes(), []byte(`"sequence":2`), []byte(`"sequence":1`), 1)} {
		if _, err := ParseProtectedHistory(data); err == nil {
			t.Fatal("torn/altered/reordered history accepted")
		}
	}
}
func TestLANProtectedJournalWriteOrFlushUnknownPoisonsWithoutReplay(t *testing.T) {
	record, _ := protectedRecordFixture(t)
	for _, kind := range []string{"partial", "write", "flush"} {
		t.Run(kind, func(t *testing.T) {
			var state protectedJournalState
			writes, flushes := 0, 0
			write := func(data []byte) (int, error) {
				writes++
				if kind == "partial" {
					return len(data) - 1, nil
				}
				if kind == "write" {
					return 0, errors.New("write unknown")
				}
				return len(data), nil
			}
			flush := func() error { flushes++; return errors.New("flush unknown") }
			entry := protectedEntry{Kind: "owner-review", Review: &record}
			if state.append(entry, write, flush) == nil || state.append(entry, write, flush) == nil || writes != 1 || !state.failed {
				t.Fatal("failed write/flush was replayed")
			}
			if kind != "flush" && flushes != 0 {
				t.Fatal("incomplete write was flushed as success")
			}
		})
	}
}
