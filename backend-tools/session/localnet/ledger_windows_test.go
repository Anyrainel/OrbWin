//go:build windows

package localnet

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestLedgerRequiresExclusiveOwnerAndAcceptedIDBeforeSettlement(t *testing.T) {
	directory := t.TempDir()
	ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if other, err := OpenNamespaceLedger(directory, "owned-daemon"); err == nil {
		other.Close()
		t.Fatal("second controller acquired a live lease")
	}
	intent := namespaceFixture()
	if err := ledger.Begin(intent); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Begin(intent); err == nil {
		t.Fatal("pending intent was overwritten")
	}
	records, err := ledger.records()
	if err != nil || len(records) != 1 || records[0].ID != "" {
		t.Fatalf("flushed intent unreadable: %+v %v", records, err)
	}
	if err := ledger.settled(records[0]); err == nil {
		t.Fatal("missing unknown create was forgotten")
	}
	id := strings.Repeat("e", 64)
	if err := ledger.Created(intent, id); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Created(intent, strings.Repeat("f", 64)); err == nil {
		t.Fatal("accepted identity was replaced")
	}
	records, err = ledger.records()
	if err != nil || records[0].ID != id {
		t.Fatal("accepted helper ID was not durable")
	}
	if err := ledger.settled(records[0]); err != nil {
		t.Fatal(err)
	}
	if entries, _ := ledger.records(); len(entries) != 0 {
		t.Fatal("settled exact ID retained its record")
	}
	ledger.Close()
	if err := ledger.Begin(intent); err == nil {
		t.Fatal("closed ledger accepted a new intent")
	}
}

func TestLedgerRetainsDamagedOrForeignRecords(t *testing.T) {
	for _, fault := range []string{"partial-header", "partial-id", "extra-line", "foreign-daemon", "wrong-filename"} {
		t.Run(fault, func(t *testing.T) {
			directory := t.TempDir()
			ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			intent := namespaceFixture()
			if err := ledger.Begin(intent); err != nil {
				t.Fatal(err)
			}
			path := ledger.path(intent.Session)
			data, _ := os.ReadFile(path)
			switch fault {
			case "partial-header":
				data = data[:len(data)/2]
			case "partial-id":
				data = append(data, []byte(`{"id":"partial`)...)
			case "extra-line":
				data = append(data, []byte("{}\n{}\n")...)
			case "foreign-daemon":
				data = []byte(strings.Replace(string(data), "owned-daemon", "foreign-daemon", 1))
			case "wrong-filename":
				if err := os.Rename(path, filepath.Join(directory, "namespace-"+strings.Repeat("f", 32)+".jsonl")); err != nil {
					t.Fatal(err)
				}
			}
			if fault != "wrong-filename" {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ledger.records(); err == nil {
				t.Fatal("damaged/foreign ownership was accepted")
			}
			intent.Session = strings.Repeat("b", 32)
			if err := ledger.Begin(intent); err == nil {
				t.Fatal("new helper intent bypassed retained damaged ownership")
			}
			entries, _ := os.ReadDir(directory)
			if len(entries) != 2 {
				t.Fatal("damaged record was removed or extra resource intent created")
			}
		})
	}
}

func TestLedgerLeaseChild(t *testing.T) {
	directory := os.Getenv("ORBWIN_LEDGER_CRASH_TEST")
	if directory == "" {
		return
	}
	ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		os.Exit(92)
	}
	intent := namespaceFixture()
	if ledger.Begin(intent) != nil || ledger.Created(intent, strings.Repeat("e", 64)) != nil {
		os.Exit(93)
	}
	os.Exit(91) // Deliberately bypass Close/defer, not a simulated successful shutdown.
}
func TestLedgerAcceptedIntentSurvivesProcessLoss(t *testing.T) {
	directory := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestLedgerLeaseChild$")
	command.Env = append(os.Environ(), "ORBWIN_LEDGER_CRASH_TEST="+directory)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := command.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 91 {
		t.Fatalf("crash fixture did not reach flushed accepted intent: %v", err)
	}
	ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		t.Fatalf("OS did not release crashed owner's lease: %v", err)
	}
	defer ledger.Close()
	records, err := ledger.records()
	if err != nil || len(records) != 1 || records[0].ID != strings.Repeat("e", 64) {
		t.Fatalf("accepted ownership lost after process crash: %+v %v", records, err)
	}
}
