package main

// Shared bounded TTY transport for ordinary exec and owned diagnostic sidecars.
import (
	"bufio"
	"errors"
	"fmt"
	"github.com/moby/moby/client"
	"io"
	"os"
	"time"
)

func interact(attached client.HijackedResponse, scanner *bufio.Scanner, output io.Writer, resize func(uint, uint) error, exit func() (int, error), monitor func() error) (int, error) {
	fmt.Fprintln(os.Stderr, "ORBWIN_SESSION_READY")
	copied := make(chan error, 1)
	commands := make(chan error, 1)
	go func() { _, err := io.Copy(output, attached.Reader); copied <- err }()
	go func() {
		for scanner.Scan() {
			next, err := decode(scanner.Bytes())
			if err != nil {
				commands <- err
				return
			}
			switch next.Kind {
			case "input":
				if len(next.Data) > 8192 {
					commands <- errors.New("input exceeds 8 KiB")
					return
				}
				if err := attached.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
					commands <- err
					return
				}
				err := writeFull(attached.Conn, next.Data)
				_ = attached.Conn.SetWriteDeadline(time.Time{})
				if err != nil {
					commands <- err
					return
				}
			case "resize":
				if err := dimensions(next.Cols, next.Rows); err != nil {
					commands <- err
					return
				}
				err := resize(next.Cols, next.Rows)
				if err != nil {
					commands <- err
					return
				}
			case "close":
				commands <- nil
				return
			default:
				commands <- errors.New("unsupported session message")
				return
			}
		}
		commands <- scanner.Err()
	}()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(time.Hour)
	defer deadline.Stop()
	for {
		select {
		case <-ticker.C:
			if monitor != nil {
				if err := monitor(); err != nil {
					return 125, err
				}
			}
		case err := <-copied:
			if err != nil {
				return 125, err
			}
			return exit()
		case err := <-commands:
			return 125, err
		case <-deadline.C:
			return 125, errors.New("terminal client reached its one-hour session limit")
		}
	}
}
