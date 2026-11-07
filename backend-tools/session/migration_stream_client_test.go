package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func TestMigrationStreamClientScopesHTTPAndHijack(t *testing.T) {
	execID := strings.Repeat("a", 64)
	var executed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch migrationAPIPath(r.URL.Path) {
		case "/_ping":
			w.Header().Set("API-Version", "1.54")
			w.Write([]byte("OK"))
		case "/info":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ID":"source"}`))
		case "/exec/" + execID + "/start":
			executed.Add(1)
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
			conn, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Type: application/vnd.docker.raw-stream\r\n\r\nfixed output\x00\xff\r\n")
			buffer.Flush()
		default:
			t.Errorf("unauthorized request reached source: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 400)
		}
	}))
	defer server.Close()
	var dials atomic.Int32
	dial := func(ctx context.Context, _, address string) (net.Conn, error) {
		if address != "127.0.0.1:0" {
			t.Errorf("unexpected SDK namespace %q", address)
		}
		dials.Add(1)
		return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(server.URL, "http://"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	scope := &migrationVolumeScope{execs: map[string]bool{execID: true}}
	cli, closeClient, err := newMigrationStreamClient(dial, func(base http.RoundTripper) http.RoundTripper { return migrationVolumeTransport{base, scope} }, scope.allow)
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != "source" {
		t.Fatalf("ordinary metadata: %v %v", info, err)
	}
	attached, err := cli.ExecAttach(ctx, execID, client.ExecAttachOptions{})
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(attached.Reader)
	attached.Close()
	if err != nil || string(output) != "fixed output\x00\xff\r\n" {
		t.Fatalf("hijack wire bytes %q: %v", output, err)
	}
	if _, err := cli.ExecAttach(ctx, strings.Repeat("b", 64), client.ExecAttachOptions{}); err == nil {
		t.Fatal("unacknowledged exec accepted")
	}
	if _, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: "forbidden"}); err == nil {
		t.Fatal("source volume mutation accepted")
	}
	if executed.Load() != 1 || dials.Load() < 2 {
		t.Fatalf("exec/dial counts %d/%d", executed.Load(), dials.Load())
	}

	readOnly, closeReadOnly, err := newMigrationStreamClient(dial, func(base http.RoundTripper) http.RoundTripper { return migrationTransport{base} }, migrationReadRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer closeReadOnly()
	if _, err := readOnly.ExecAttach(ctx, execID, client.ExecAttachOptions{}); err == nil {
		t.Fatal("read-only source accepted exec")
	}
	if executed.Load() != 1 {
		t.Fatal("read-only exec reached source")
	}
}
