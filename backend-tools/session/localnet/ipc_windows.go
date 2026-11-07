//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/sdkself"
)

// The pipe grants this Windows user access only. HTTP listeners never dispatch
// administrative requests. Each connection accepts one bounded typed frame.
func ServeServiceIPC(ctx context.Context, runtime *ServiceRuntime, home, installation string) error {
	name, err := servicePipeName(home, installation)
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	listener, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + user.User.Sid.String() + ")", InputBufferSize: ipcRequestBudget + 4, OutputBufferSize: 64 << 10})
	if err != nil {
		return err
	}
	return serveServiceIPCListener(ctx, runtime, listener)
}

// One loop serves both the ordinary native creator and the opaque cold lease.
// Return means every connection worker has joined. The listener's actual owner
// retains responsibility for observing its positive close result.
func serveServiceIPCListener(ctx context.Context, runtime *ServiceRuntime, listener net.Listener) error {
	if listener == nil {
		return errors.New("owned local-services listener and runtime required")
	}
	defer listener.Close()
	if ctx == nil || runtime == nil {
		return errors.New("owned local-services listener and runtime required")
	}
	var workers sync.WaitGroup
	slots := make(chan struct{}, 16)
	var mu sync.Mutex
	connections := map[net.Conn]struct{}{}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-runtime.done:
		case <-stop:
			return
		}
		listener.Close()
		mu.Lock()
		for connection := range connections {
			connection.Close()
		}
		mu.Unlock()
	}()
	defer func() {
		close(stop)
		mu.Lock()
		for connection := range connections {
			connection.Close()
		}
		mu.Unlock()
		workers.Wait()
	}()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			select {
			case <-runtime.done:
				return nil
			default:
				return acceptErr
			}
		}
		select {
		case slots <- struct{}{}:
		default:
			connection.Close()
			continue
		}
		mu.Lock()
		connections[connection] = struct{}{}
		mu.Unlock()
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { connection.Close(); mu.Lock(); delete(connections, connection); mu.Unlock(); <-slots }()
			if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return
			}
			data, err := readServiceFrame(connection, ipcRequestBudget)
			if err != nil {
				return
			}
			var request ServiceRequest
			if err := strictJSON(data, &request); err != nil {
				request = ServiceRequest{}
			}
			requestCtx, cancel := context.WithTimeout(ctx, ServiceOperationTimeout(request))
			defer cancel()
			response := dispatchServiceRequest(requestCtx, runtime, data)
			if err = connection.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return
			}
			replyBytes := encodeServiceResponse(response)
			disposition := stopObservationMode(request.Operation, request.ExpectedServer != nil, response.Error)
			if response.shutdown && disposition == stopReplyRetain {
				_ = writeServiceFrame(connection, replyBytes, ipcResponseBudget)
				return // e.g. post-dispatch Snapshot failure: expected stop remains uncancelled
			}
			if response.shutdown && disposition == stopReplyObserve {
				observed := observeStopReply(replyBytes,
					func() error { return writeServiceFrame(connection, replyBytes, ipcResponseBudget) },
					func() ([]byte, error) {
						if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
							return nil, err
						}
						return readServiceFrame(connection, stopObservationBudget)
					})
				if observed != nil {
					return
				} // no cancel: SAME original service remains retained/live
				runtime.cancel()
			} else {
				_ = writeServiceFrame(connection, replyBytes, ipcResponseBudget)
				if response.shutdown {
					runtime.cancel()
				} // legacy omitted-expected behavior
			}
		}()
	}
}
func CallServiceIPC(ctx context.Context, home, installation string, request ServiceRequest) (response ServiceResponse, err error) {
	return dialAuthenticatedServiceIPC(ctx, home, installation, request, func(ctx context.Context, c net.Conn) (servicePeer, error) {
		lease, err := sdkself.HoldPeer(ctx, c)
		if lease == nil {
			return nil, errors.New("native SDK pipe peer unavailable")
		}
		return lease, err
	})
}

// Private seam for the typed-frame pipe fixture. Production enters only above,
// using the actual native SDK capability, before writing any request bytes.
func dialAuthenticatedServiceIPC(ctx context.Context, home, installation string, request ServiceRequest, authenticate func(context.Context, net.Conn) (servicePeer, error)) (response ServiceResponse, err error) {
	if ctx == nil || authenticate == nil {
		return response, errors.New("bounded native SDK peer context required")
	}
	name, err := servicePipeName(home, installation)
	if err != nil {
		return response, err
	}
	data, err := json.Marshal(request)
	if err != nil || len(data) > ipcRequestBudget {
		return response, errors.New("local-services request exceeds its budget")
	}
	connection, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return response, ErrServiceNotRunning
		}
		return response, err
	}
	return callAuthenticatedService(ctx, connection, request, data, authenticate)
}
