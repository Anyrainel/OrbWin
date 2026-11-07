// Package landatagram preserves UDP packet boundaries across an owned exec
// stream. It authorizes neither LAN peers nor namespace/helper ownership.
package landatagram

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"
)

const MaxPayload = 65507
const ReadBudget = 30 * time.Second
const WriteBudget = 5 * time.Second

var ErrOversize = errors.New("UDP payload exceeds IPv4 bound")
var ErrShortBuffer = errors.New("UDP packet dropped: destination buffer too small")

// Connection deliberately has no stream Read/Write methods. Empty packets are
// (0,nil), while EOF is (0,io.EOF). Close interrupts blocked operations.
type Connection interface {
	ReadPacket([]byte) (int, error)
	WritePacket([]byte) (int, error)
	Close() error
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}
type Transport interface {
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

type Codec struct {
	stream                      Transport
	now                         func() time.Time
	readMu, writeMu, deadlineMu sync.Mutex
	readDeadline, writeDeadline time.Time
	buffer                      [MaxPayload]byte
	once                        sync.Once
	closeErr                    error
}

func New(stream Transport) *Codec                            { return newCodec(stream, time.Now) }
func newCodec(stream Transport, now func() time.Time) *Codec { return &Codec{stream: stream, now: now} }
func bounded(now, requested time.Time, budget time.Duration) time.Time {
	limit := now.Add(budget)
	if !requested.IsZero() && requested.Before(limit) {
		return requested
	}
	return limit
}
func (c *Codec) Close() error { c.once.Do(func() { c.closeErr = c.stream.Close() }); return c.closeErr }
func (c *Codec) SetDeadline(t time.Time) error {
	return errors.Join(c.SetReadDeadline(t), c.SetWriteDeadline(t))
}
func (c *Codec) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	return c.stream.SetReadDeadline(bounded(c.now(), t, ReadBudget))
}
func (c *Codec) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.writeDeadline = t
	return c.stream.SetWriteDeadline(bounded(c.now(), t, WriteBudget))
}
func (c *Codec) ReadPacket(dst []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	c.deadlineMu.Lock()
	err := c.stream.SetReadDeadline(bounded(c.now(), c.readDeadline, ReadBudget))
	c.deadlineMu.Unlock()
	if err != nil {
		_ = c.Close()
		return 0, err
	}
	n, err := ReadFrame(c.stream, c.buffer[:])
	if err != nil {
		_ = c.Close()
		return 0, err
	}
	if len(dst) < n {
		return 0, ErrShortBuffer
	}
	copy(dst, c.buffer[:n])
	return n, nil
}
func (c *Codec) WritePacket(packet []byte) (int, error) {
	if len(packet) > MaxPayload {
		return 0, ErrOversize
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.deadlineMu.Lock()
	err := c.stream.SetWriteDeadline(bounded(c.now(), c.writeDeadline, WriteBudget))
	c.deadlineMu.Unlock()
	if err == nil {
		err = WriteFrame(c.stream, packet)
	}
	if err != nil {
		_ = c.Close()
		return 0, err
	}
	return len(packet), nil
}

// ReadFrame never allocates from an untrusted length. A small buffer is a caller
// programming error here; Codec always supplies MaxPayload and drops at its API.
func ReadFrame(reader io.Reader, buffer []byte) (int, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n > MaxPayload {
		return 0, ErrOversize
	}
	if int(n) > len(buffer) {
		return 0, ErrShortBuffer
	}
	if _, err := io.ReadFull(reader, buffer[:int(n)]); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return 0, err
	}
	return int(n), nil
}
func WriteFrame(writer io.Writer, packet []byte) error {
	if len(packet) > MaxPayload {
		return ErrOversize
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(packet)))
	if err := writeAll(writer, header[:]); err != nil {
		return err
	}
	return writeAll(writer, packet)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if n < 0 || n > len(b) {
			return io.ErrShortWrite
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
