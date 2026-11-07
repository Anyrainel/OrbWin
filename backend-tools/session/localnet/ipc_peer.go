package localnet

import (
	"context"
	"errors"
	"net"
	"orbwin.local/session/internal/sdkself"
	"sync"
	"time"
)

// Production supplies only sdkself.HoldPeer, whose capability comes from the
// actual connected pipe handle. Private seam permits pure no-dispatch faults.
type servicePeer interface {
	Identity() (sdkself.Identity, error)
	Close() error
}

func callAuthenticatedService(ctx context.Context, connection net.Conn, request ServiceRequest, data []byte, authenticate func(context.Context, net.Conn) (servicePeer, error)) (response ServiceResponse, err error) {
	if ctx == nil || connection == nil || authenticate == nil {
		return response, errors.New("native SDK peer context required")
	}
	var closeOnce sync.Once
	var closeErr error
	var peer servicePeer
	closeConnection := func() { closeOnce.Do(func() { closeErr = connection.Close() }) }
	defer func() {
		closeConnection()
		if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			response = ServiceResponse{}
			err = errors.New("native SDK pipe close unknown")
		}
		if peer != nil {
			if peer.Close() != nil {
				response = ServiceResponse{}
				err = errors.New("native SDK peer lease close unknown")
			}
		}
	}()
	peer, err = authenticate(ctx, connection)
	if err != nil || peer == nil {
		return ServiceResponse{}, errors.New("native SDK pipe peer unavailable; request not sent")
	}
	closedOnCancel := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { defer close(closedOnCancel); closeConnection() })
	defer func() {
		if !stop() {
			<-closedOnCancel
		}
	}() // Join a dispatched cancellation close before releasing the peer lease.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(ServiceOperationTimeout(request) + 5*time.Second)
	}
	if err = connection.SetDeadline(deadline); err != nil {
		return response, err
	}
	identity, err := peer.Identity()
	if err != nil || !identity.Valid() || ctx.Err() != nil {
		return ServiceResponse{}, errors.New("native SDK pipe peer changed; request not sent")
	}
	if request.ExpectedServer != nil && compareExpectedServer(request.ExpectedServer, identity.PID, identity.CreationTime) != nil {
		return ServiceResponse{}, errRestricted // Actual connected peer was checked; zero request bytes sent.
	}
	if err = writeServiceFrame(connection, data, ipcRequestBudget); err != nil {
		return response, err
	}
	reply, err := readServiceFrame(connection, ipcResponseBudget)
	if err != nil {
		return response, err
	}
	if strictJSON(reply, &response) != nil || response.Version != 1 {
		return ServiceResponse{}, errors.New("native SDK response malformed; outcome unconfirmed")
	}
	after, err := peer.Identity()
	if err != nil || !sdkself.SameIdentity(identity, after) {
		return ServiceResponse{}, errors.New("native SDK reply peer changed; outcome unconfirmed")
	}
	if response.State != nil && (response.State.SelfIdentity == nil || !sdkself.SameIdentity(*response.State.SelfIdentity, after)) {
		return ServiceResponse{}, errors.New("native SDK reply identity differs; outcome unconfirmed")
	}
	if stopObservationRequired(request.Operation, request.ExpectedServer != nil, response.Error) {
		// Native peer/reply checks above complete while server remains alive.
		// A write error is uncertain: caller must not retry or restart.
		if err = writeServiceFrame(connection, stopReplyObservation(reply), stopObservationBudget); err != nil {
			return ServiceResponse{}, errors.New("local-services stop observation failed; outcome unconfirmed")
		}
	}
	return response, nil
}
