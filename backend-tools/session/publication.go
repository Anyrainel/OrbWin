package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

type publication struct {
	ContainerID string `json:"containerId"`
	Port        uint16 `json:"port"`
	Protocol    string `json:"protocol"`
	Published   bool   `json:"published"`
}

func publicationSnapshot(output io.Writer, daemonID string) error {
	if daemonID == "" || len(daemonID) > 256 || strings.ContainsAny(daemonID, "\x00\r\n") {
		return errors.New("recorded daemon identity required")
	}
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != daemonID || info.Info.OSType != "linux" {
		return errors.New("publication endpoint does not match the recorded owned Linux daemon")
	}
	request, err := http.NewRequestWithContext(ctx, "GET", "http://docker/orbwin/port-status", nil)
	if err != nil {
		return err
	}
	// Reuse the SDK's fixed named-pipe dialer for this unversioned extension;
	// this transport has no environment proxy or redirect to another endpoint.
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return cli.Dialer()(ctx) }}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("owned host-publication status is unavailable; inspect backend diagnostics")
	}
	bytes, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil {
		return err
	}
	if len(bytes) > 2<<20 {
		return errors.New("publication snapshot exceeds 2 MiB")
	}
	var entries []publication
	if err = json.Unmarshal(bytes, &entries); err != nil {
		return err
	}
	if len(entries) > 2048 {
		return errors.New("publication snapshot exceeds 2048 entries")
	}
	for _, entry := range entries {
		if len(entry.ContainerID) != 64 || entry.Port == 0 || (entry.Protocol != "tcp" && entry.Protocol != "udp") {
			return errors.New("invalid host-publication record")
		}
		if _, err = hex.DecodeString(entry.ContainerID); err != nil {
			return errors.New("invalid host-publication container ID")
		}
	}
	return json.NewEncoder(output).Encode(entries)
}
