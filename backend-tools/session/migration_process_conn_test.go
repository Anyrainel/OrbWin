package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestMigrationProcessWorker(t *testing.T) {
	mode := os.Getenv("ORBWIN_PROCESS_FIXTURE")
	if mode == "" {
		t.Skip("child-only process fixture")
	}
	switch mode {
	case "echo":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "burst":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{0, 255, 13, 10}, 256<<10))
	case "fail":
		_, _ = os.Stdout.Write([]byte("prefix"))
		os.Exit(42)
	case "wait":
		time.Sleep(time.Minute)
	default:
		os.Exit(43)
	}
	os.Exit(0)
}
func migrationProcessFixture(t *testing.T, ctx context.Context, mode string) *migrationProcessConn {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestMigrationProcessWorker$")
	command.Env = append(os.Environ(), "ORBWIN_PROCESS_FIXTURE="+mode)
	conn, err := startMigrationProcess(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
func TestMigrationProcessConnDrainsExitedChildAndPreservesBytes(t *testing.T) {
	conn := migrationProcessFixture(t, context.Background(), "burst")
	data, err := io.ReadAll(conn)
	if err != nil || !bytes.Equal(data, bytes.Repeat([]byte{0, 255, 13, 10}, 256<<10)) {
		t.Fatalf("lost/translated child output: %d %v", len(data), err)
	}
	failure := migrationProcessFixture(t, context.Background(), "fail")
	data, err = io.ReadAll(failure)
	if string(data) != "prefix" || err == nil {
		t.Fatalf("child failure turned into clean EOF: %q %v", data, err)
	}
}
func TestMigrationProcessConnDeadlinesResetAndCancellationReaps(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := migrationProcessFixture(t, ctx, "echo")
	if err := conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("deadline not honored: %v", err)
	}
	conn.SetReadDeadline(time.Time{})
	payload := bytes.Repeat([]byte{0, 255, 13, 10}, 4096)
	written := make(chan error, 1)
	go func() { _, err := conn.Write(payload); written <- err }()
	actual := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, actual); err != nil || !bytes.Equal(actual, payload) {
		t.Fatalf("deadline reset lost stream: %v", err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-conn.done:
	case <-time.After(4 * time.Second):
		t.Fatal("owned child not reaped")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationProcessConnBlockedWriteIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := migrationProcessFixture(t, ctx, "wait")
	conn.SetWriteDeadline(time.Now().Add(30 * time.Millisecond))
	if _, err := conn.Write(bytes.Repeat([]byte("x"), 1<<20)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("blocked child input ignored write deadline: %v", err)
	}
	cancel()
	conn.Close()
	select {
	case <-conn.done:
	case <-time.After(4 * time.Second):
		t.Fatal("blocked child was not reaped")
	}
}
