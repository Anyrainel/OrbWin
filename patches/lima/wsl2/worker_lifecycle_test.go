// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package wsl2

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkerErrorReportNeverWaitsForReceiver(t *testing.T) {
	reports := make(chan error, 2)
	failure := errors.New("original result")
	reportDriverError(reports, failure)
	reportDriverError(reports, failure)
	reportDriverError(reports, failure)
	reportDriverError(nil, failure)
	if len(reports) != 2 {
		t.Fatal("bounded reports")
	}
}
func TestWorkerJoinRequiresActualDone(t *testing.T) {
	var w driverWorkers
	r := &driverCommand{done: make(chan struct{}), result: errors.New("original failure"), beforeStop: true}
	w.commands = append(w.commands, r)
	w.seal()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(w.join(ctx), context.Canceled) {
		t.Fatal("notification substituted for join")
	}
	close(r.done)
	if !errors.Is(w.join(context.Background()), r.result) {
		t.Fatal("original failure lost")
	}
	if err := w.launch(nil, false, nil, ""); err == nil {
		t.Fatal("new admission after seal")
	}
}

func TestOriginalPrearmedStartErrorIsReportedAndJoined(t *testing.T) {
	original := errors.New("prearmed resolution failure")
	script := filepath.Join(t.TempDir(), "source-script.sh")
	if err := os.WriteFile(script, []byte("# source fixture only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := &exec.Cmd{Path: "never-executed", Err: original}
	var w driverWorkers
	reports := make(chan error, 1)
	if err := w.launch(cmd, false, reports, script); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-reports:
		if got != original {
			t.Fatalf("original error lost: %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("known prebirth error not reported")
	}
	if _, err := os.Stat(script); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned source script not removed: %v", err)
	}
	if cmd.Process != nil {
		t.Fatal("prearmed error created process")
	}
	w.seal()
	if !errors.Is(w.join(context.Background()), original) {
		t.Fatal("known prebirth did not settle")
	}
	if w.unknown {
		t.Fatal("known prebirth fenced")
	}
}
func TestSettledDiagnosticIsBounded(t *testing.T) {
	text := strings.Repeat("a", 5000) + "tail"
	got := boundedDriverOutput([]byte(text))
	if len(got) != 4096 || !strings.HasSuffix(got, "tail") {
		t.Fatal("diagnostic tail lost/bound exceeded")
	}
	var r driverCommand
	if err := r.removeScript(); err != nil {
		t.Fatal(err)
	}
}

func TestNonemptySourceDirectoryIsNotRecursivelyRemoved(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "script-role")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "preserved-source.txt")
	const text = "source fixture remains"
	if err := os.WriteFile(child, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	r := driverCommand{scriptPath: dir}
	if err := r.removeScript(); err == nil {
		t.Fatal("nonempty directory removed")
	}
	if r.scriptPath != dir {
		t.Fatal("failed cleanup lost original role")
	}
	data, err := os.ReadFile(child)
	if err != nil || string(data) != text {
		t.Fatalf("recursive cleanup affected source: %v", err)
	}
}

func TestConversionFailurePreservesErrorAndNonemptySourceDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "conversion-script-role")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "preserved-source.txt")
	const text = "do not recursively remove source"
	if err := os.WriteFile(child, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	original := errors.New("original conversion failure")
	err := cleanupDriverScriptFailure(dir, original)
	if !errors.Is(err, original) {
		t.Fatal("original conversion failure lost")
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != dir {
		t.Fatal("cleanup failure/original role not retained")
	}
	data, readErr := os.ReadFile(child)
	if readErr != nil || string(data) != text {
		t.Fatalf("recursive cleanup affected source: %v", readErr)
	}
	script := filepath.Join(t.TempDir(), "closed-source-script")
	if err := os.WriteFile(script, []byte("# source fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cleanupDriverScriptFailure(script, original); !errors.Is(err, original) {
		t.Fatal("successful cleanup lost conversion refusal")
	}
	if _, err := os.Stat(script); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("closed script not removed")
	}
}
