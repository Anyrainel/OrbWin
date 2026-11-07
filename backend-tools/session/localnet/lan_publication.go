package localnet

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

// validateLANPublication uses only the configured owned daemon's raw dialer;
// custom source paths must not be sent through Docker's versioned API router.
// The current pinned upstream lacks this additive endpoint: 404 remains an
// explicit gate, never a fallback to container-list ports or guessed epochs.
func validateLANPublication(ctx context.Context, cli *client.Client, daemon string, want lanlease.Publication) (time.Time, error) {
	if cli == nil || daemon == "" || want.Daemon != daemon || !fullID(want.ContainerID) {
		return time.Time{}, ErrLANAdapterUnavailable
	}
	data, e := readLANPublication(ctx, cli, daemon, want.ContainerID)
	if e != nil {
		return time.Time{}, e
	}
	return lanhelper.ReadPublication(data, want, time.Now())
}

var errLANPublicationUnavailable = errors.New("required publication-v2 endpoint unavailable")

func readLANPublication(ctx context.Context, cli *client.Client, daemon, containerID string) ([]byte, error) {
	if cli == nil || daemon == "" || !fullID(containerID) {
		return nil, ErrLANAdapterUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	verify := func() error {
		info, e := cli.Info(ctx, client.InfoOptions{})
		if e != nil || info.Info.ID != daemon || info.Info.OSType != "linux" {
			return errors.New("LAN publication daemon identity differs")
		}
		return nil
	}
	if e := verify(); e != nil {
		return nil, e
	}
	conn, e := cli.Dialer()(ctx)
	if e != nil {
		return nil, errors.New("owned publication source unavailable")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if conn.SetDeadline(deadline) != nil {
		return nil, errors.New("publication source deadline unavailable")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, "http://orbwin-owned/orbwin/port-status/v2/"+containerID, nil)
	if e != nil {
		return nil, e
	}
	req.Close = true
	if e = req.Write(conn); e != nil {
		return nil, errors.New("publication source request failed")
	}
	response, e := http.ReadResponse(bufio.NewReader(io.LimitReader(conn, 2*lanhelper.PublicationBudget)), req)
	if e != nil {
		return nil, errors.New("publication source response invalid")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusNotImplemented {
		return nil, errLANPublicationUnavailable
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("owned publication source refused")
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, lanhelper.PublicationBudget+1))
	if e != nil {
		return nil, errors.New("publication source body incomplete")
	}
	_, _, e = lanhelper.ParsePublication(data, containerID, time.Now())
	if e != nil {
		return nil, e
	}
	if e = verify(); e != nil {
		return nil, e
	}
	return data, nil
}
