// Browser qualification fixture only. One nonce route, bounded lifetime and
// headers; no workload data, writes, external requests, cookies or redirects.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	if len(os.Args) != 2 || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(os.Args[1]) {
		os.Exit(125)
	}
	nonce := os.Args[1]
	var logMu sync.Mutex
	emit := func(value any) {
		logMu.Lock()
		defer logMu.Unlock()
		_ = json.NewEncoder(os.Stdout).Encode(value)
	}
	var active atomic.Int32
	server := &http.Server{
		Addr: ":8080", ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 3 * time.Second, MaxHeaderBytes: 4096,
		ErrorLog: log.New(io.Discard, "", 0),
		ConnState: func(conn net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				if active.Add(1) > 16 {
					_ = conn.Close()
				}
			case http.StateClosed, http.StateHijacked:
				active.Add(-1)
			}
		},
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		host = strings.ToLower(host)
		allowed := err == nil && (host == "localhost" || host == "network-check.orbwin.localhost" || host == "127.0.0.1")
		matched := allowed && r.URL.Path == "/"+nonce && r.URL.RawQuery == "" && (r.Method == "GET" || r.Method == "HEAD")
		// Whitelist classifications, never raw URI/header/remote-address values.
		if !allowed {
			host = "other"
		}
		method := r.Method
		if method != "GET" && method != "HEAD" {
			method = "other"
		}
		emit(struct {
			Kind, Host, Method string
			Matched            bool
		}{"request", host, method, matched})
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !matched {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if method == "GET" {
			_, _ = fmt.Fprintf(w, "<!doctype html><title>OrbWin localhost proof</title><h1>OrbWin localhost proof</h1><p id=proof>%s</p>", nonce)
		}
	})
	listener, err := net.Listen("tcp4", server.Addr)
	if err != nil {
		os.Exit(125)
	}
	timer := time.AfterFunc(180*time.Second, func() { _ = server.Close() })
	defer timer.Stop()
	emit(struct{ Kind string }{"ready"})
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		os.Exit(125)
	}
}
