package main

// A migration-only byte stream over an owned child. net.Pipe supplies real,
// resettable read/write deadlines and backpressure; the child receives the
// unchanged Docker wire bytes through upstream `docker system dial-stdio`.
// This is not a published Docker endpoint or interactive-stdin transport.
import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"
)

type migrationProcessConn struct {
	net.Conn
	reader      *bufio.Reader
	command     *exec.Cmd
	relay       net.Conn
	input       *os.File
	output      *os.File
	done        chan struct{}
	pumps       sync.WaitGroup
	closeOnce   sync.Once
	stopMu      sync.Mutex
	stopContext func() bool
	waitErr     error // written before done closes; readers first receive done
}

func startMigrationProcess(ctx context.Context, command *exec.Cmd) (*migrationProcessConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outputRead, outputWrite, err := os.Pipe()
	if err != nil {
		inputRead.Close()
		inputWrite.Close()
		return nil, err
	}
	// Manually own the pipes: exec.Wait must not close the reader before the
	// relay has drained buffered output from a child which already exited.
	command.Stdin = inputRead
	command.Stdout = outputWrite
	command.WaitDelay = 2 * time.Second
	if command.Stderr == nil {
		command.Stderr = io.Discard
	}
	if err := command.Start(); err != nil {
		inputRead.Close()
		inputWrite.Close()
		outputRead.Close()
		outputWrite.Close()
		return nil, err
	}
	inputRead.Close()
	outputWrite.Close()
	local, relay := net.Pipe()
	conn := &migrationProcessConn{Conn: local, command: command, relay: relay, input: inputWrite, output: outputRead, done: make(chan struct{})}
	go func() { conn.waitErr = command.Wait(); close(conn.done) }()
	conn.pumps.Add(2)
	go func() { defer conn.pumps.Done(); _, _ = io.Copy(inputWrite, relay); inputWrite.Close() }()
	go func() { defer conn.pumps.Done(); _, _ = io.Copy(relay, outputRead); outputRead.Close(); relay.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	conn.stopMu.Lock()
	conn.stopContext = stop
	conn.stopMu.Unlock()
	return conn, nil
}

func (conn *migrationProcessConn) Read(data []byte) (int, error) {
	var n int
	var err error
	if conn.reader != nil {
		n, err = conn.reader.Read(data)
	} else {
		n, err = conn.Conn.Read(data)
	}
	if err != io.EOF {
		return n, err
	}
	select {
	case <-conn.done:
		if conn.waitErr != nil {
			return n, fmt.Errorf("selected migration connection exited before completion: %w", conn.waitErr)
		}
		return n, io.EOF
	case <-time.After(2 * time.Second):
		_ = conn.Close()
		return n, errors.New("selected migration connection did not settle after EOF")
	}
}

func (conn *migrationProcessConn) Close() error {
	conn.closeOnce.Do(func() {
		conn.stopMu.Lock()
		stop := conn.stopContext
		conn.stopMu.Unlock()
		if stop != nil {
			stop()
		}
		conn.Conn.Close()
		conn.relay.Close()
		conn.input.Close()
		conn.output.Close()
		select {
		case <-conn.done:
		case <-time.After(2 * time.Second):
			_ = conn.command.Process.Kill()
			<-conn.done
		}
		conn.pumps.Wait()
	})
	return nil
}
