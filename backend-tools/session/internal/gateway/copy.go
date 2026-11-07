package gateway

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
)

const StreamIdleTimeout = 2 * time.Minute

type activeIO struct {
	connection net.Conn
	idle       time.Duration
}

func (stream *activeIO) Read(data []byte) (int, error) {
	if err := stream.connection.SetDeadline(time.Now().Add(stream.idle)); err != nil {
		return 0, err
	}
	return stream.connection.Read(data)
}
func (stream *activeIO) Write(data []byte) (int, error) {
	if err := stream.connection.SetDeadline(time.Now().Add(stream.idle)); err != nil {
		return 0, err
	}
	return stream.connection.Write(data)
}

// Preserve both independent write-half closures over TCP or TLS. No interface
// fast path can bypass the idle deadline wrapper. Cancellation/error closes and
// joins both copy workers; request EOF still permits the complete response.
func CopyDuplex(ctx context.Context, left, right net.Conn) error {
	return copyDuplex(ctx, left, right, StreamIdleTimeout)
}
func copyDuplex(ctx context.Context, left, right net.Conn, idle time.Duration) error {
	done := make(chan error, 2)
	copyHalf := func(destination, source net.Conn) {
		_, err := io.Copy(&activeIO{destination, idle}, &activeIO{source, idle})
		half, ok := destination.(interface{ CloseWrite() error })
		if !ok {
			err = errors.Join(err, errors.New("gateway stream lacks write-half closure"))
		} else {
			err = errors.Join(err, half.CloseWrite())
		}
		done <- err
	}
	go copyHalf(left, right)
	go copyHalf(right, left)
	for completed := 0; completed < 2; completed++ {
		select {
		case err := <-done:
			if err != nil {
				left.Close()
				right.Close()
				for remaining := completed + 1; remaining < 2; remaining++ {
					<-done
				}
				return err
			}
		case <-ctx.Done():
			left.Close()
			right.Close()
			for remaining := completed; remaining < 2; remaining++ {
				<-done
			}
			return ctx.Err()
		}
	}
	return nil
}
