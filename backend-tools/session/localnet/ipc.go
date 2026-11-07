package localnet

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
)

const ipcRequestBudget = settingsBudget + 4096
const MaxServiceRequestBytes = ipcRequestBudget
const ipcResponseBudget = 1 << 20

var ErrServiceNotRunning = errors.New("local-services process is not running")

type ServiceRequest struct {
	Version         int              `json:"version"`
	ExpectedServer  *ExpectedServer  `json:"expectedServer,omitempty"`
	Operation       string           `json:"operation"`
	Revision        uint64           `json:"revision,omitempty"`
	Settings        *ServiceSettings `json:"settings,omitempty"`
	RootSHA256      string           `json:"rootSha256,omitempty"`
	NewRootSHA256   string           `json:"newRootSha256,omitempty"`
	Ports           *ServicePorts    `json:"ports,omitempty"`
	WSLReview       *WSLTrustReview  `json:"wslReview,omitempty"`
	LANSelection    *LANSelection    `json:"lanSelection,omitempty"`
	LANReviewID     string           `json:"lanReviewId,omitempty"`
	LANReviewSHA256 string           `json:"lanReviewSha256,omitempty"`
	LANOperationID  string           `json:"lanOperationId,omitempty"`
	LANLeaseSHA256  string           `json:"lanLeaseSha256,omitempty"`
}

func ServiceOperationTimeout(request ServiceRequest) time.Duration {
	switch request.Operation {
	case "wsl-trust-status", "wsl-trust-install", "wsl-trust-remove":
		return 145 * time.Second
	default:
		return 40 * time.Second
	}
}

type ServiceResponse struct {
	Version               int                `json:"version"`
	RestrictedStateSHA256 string             `json:"restrictedStateSha256,omitempty"`
	State                 *RuntimeSnapshot   `json:"state,omitempty"`
	Authority             *AuthorityInfo     `json:"authority,omitempty"`
	Error                 string             `json:"error,omitempty"`
	LANOptions            *LANOptions        `json:"lanOptions,omitempty"`
	LANReview             *LANReview         `json:"lanReview,omitempty"`
	LANReconciliation     *LANReconciliation `json:"lanReconciliation,omitempty"`
	shutdown              bool
}

func servicePipeName(home, installation string) (string, error) {
	if !filepath.IsAbs(home) || len(home) > 1024 || len(installation) < 32 || len(installation) > 36 || strings.ContainsAny(home+installation, "\x00\r\n") {
		return "", errors.New("canonical local home and installation identity required")
	}
	hash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(home)) + "\x00" + installation))
	return `\\.\pipe\OrbWinLocalServices-` + hex.EncodeToString(hash[:16]), nil
}
func readServiceFrame(reader io.Reader, budget uint32) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > budget {
		return nil, errors.New("local-services message exceeds its frame budget")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(reader, data)
	return data, err
}
func writeServiceFrame(writer io.Writer, data []byte, budget int) error {
	if len(data) == 0 || len(data) > budget {
		return errors.New("local-services reply exceeds its frame budget")
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(data)))
	for _, part := range [][]byte{header[:], data} {
		for len(part) > 0 {
			written, err := writer.Write(part)
			if err != nil {
				return err
			}
			if written <= 0 || written > len(part) {
				return io.ErrShortWrite
			}
			part = part[written:]
		}
	}
	return nil
}
func dispatchServiceRequest(ctx context.Context, runtime *ServiceRuntime, data []byte) ServiceResponse {
	response := ServiceResponse{Version: 1}
	var request ServiceRequest
	err := strictJSON(data, &request)
	if err == nil {
		var members map[string]json.RawMessage
		_ = json.Unmarshal(data, &members)
		if strings.HasPrefix(request.Operation, "lan-") {
			err = validateLANIPC(data, request.Operation)
		} else {
			for _, name := range []string{"lanSelection", "lanReviewId", "lanReviewSha256", "lanOperationId", "lanLeaseSha256"} {
				if _, exists := members[name]; exists {
					err = errors.New("LAN fields are only accepted by their exact operation")
					break
				}
			}
		}
	}
	if err == nil && request.Version != 1 {
		err = errors.New("unsupported local-services protocol version")
	}
	if err == nil && request.Operation != "configure" && request.Settings != nil {
		err = errors.New("settings are only accepted by configure")
	}
	rotationOperation := request.Operation == "rotate-authority" || request.Operation == "resume-authority-rotation" || request.Operation == "cancel-authority-rotation"
	wslOperation := request.Operation == "wsl-trust-status" || request.Operation == "wsl-trust-install" || request.Operation == "wsl-trust-remove"
	wslOperation = wslOperation || request.Operation == "retire-wsl-trust-receipt"
	if err == nil && !wslOperation && request.WSLReview != nil {
		err = errors.New("selected distro review is only accepted by WSL trust operations")
	}
	if err == nil && !rotationOperation && !wslOperation && request.Operation != "adopt-authority" && request.Operation != "trust-status" && request.Operation != "trust-install" && request.Operation != "trust-remove" && request.RootSHA256 != "" {
		err = errors.New("this operation does not accept a reviewed root fingerprint")
	}
	if err == nil && request.Operation != "rotate-authority" && request.Ports != nil {
		err = errors.New("port pair is only accepted by authority rotation")
	}
	if err == nil && request.Operation != "resume-authority-rotation" && request.Operation != "cancel-authority-rotation" && request.NewRootSHA256 != "" {
		err = errors.New("new root fingerprint is only accepted when resolving retained rotation")
	}
	if err == nil {
		enabled := request.Settings != nil && request.Settings.Enabled
		if runtime.opts.Restriction != nil && (!restrictedOperation(request.Operation, enabled) || request.Operation != "snapshot" && request.ExpectedServer == nil) {
			err = errRestricted
		}
		if request.ExpectedServer != nil && runtime.checkExpectedServer(request.ExpectedServer) != nil {
			err = errRestricted
		}
	}
	if err == nil {
		switch request.Operation {
		case "lan-review":
			if request.LANSelection == nil {
				err = errors.New("complete LAN selection required")
			} else {
				var review LANReview
				review, err = runtime.ReviewLAN(ctx, request.Revision, *request.LANSelection)
				if review.ReviewID != "" {
					response.LANReview = &review
				}
			}
		case "lan-remove-review", "lan-settle-remove-review":
			var reply runtimeReply
			reply, err = runtime.request(ctx, runtimeCommand{kind: request.Operation, lanLeaseSHA: request.LANLeaseSHA256})
			response.LANReview = reply.lanReview
		case "lan-activate", "lan-remove", "lan-settle-remove", "lan-cleanup", "lan-cancel", "lan-inspect", "lan-settle-helper":
			_, err = runtime.request(ctx, runtimeCommand{kind: request.Operation, revision: request.Revision, lanReviewID: request.LANReviewID, lanReviewSHA: request.LANReviewSHA256, lanOperationID: request.LANOperationID, lanLeaseSHA: request.LANLeaseSHA256})
		case "lan-reconcile-review":
			var reply runtimeReply
			reply, err = runtime.request(ctx, runtimeCommand{kind: "lan-reconcile-review", lanReviewID: request.LANReviewID, lanReviewSHA: request.LANReviewSHA256})
			response.LANReconciliation = reply.lanReconciliation
		case "lan-cleanup-review":
			var reply runtimeReply
			reply, err = runtime.request(ctx, runtimeCommand{kind: request.Operation, lanReviewID: request.LANReviewID, lanReviewSHA: request.LANReviewSHA256})
			response.LANReview = reply.lanReview
		case "lan-options":
			var options LANOptions
			options, err = runtime.ReadLANOptions(ctx)
			if options.Version == 1 {
				response.LANOptions = &options
			}
		case "wsl-trust-distributions", "wsl-trust-receipts":
			err = runtime.SelectedWSLTrust(ctx, request.Operation, 0, "", WSLTrustReview{})
		case "wsl-trust-status", "wsl-trust-install", "wsl-trust-remove", "retire-wsl-trust-receipt":
			if request.WSLReview == nil {
				err = errors.New("selected WSL registration review required")
			} else {
				err = runtime.SelectedWSLTrust(ctx, request.Operation, request.Revision, request.RootSHA256, *request.WSLReview)
			}
		case "rotate-authority", "resume-authority-rotation", "cancel-authority-rotation":
			if request.Operation == "rotate-authority" && request.Ports == nil {
				err = errors.New("reviewed target port pair required for rotation")
			} else {
				ports := ServicePorts{}
				if request.Ports != nil {
					ports = *request.Ports
				}
				err = runtime.RotateAuthority(ctx, request.Operation, request.Revision, request.RootSHA256, request.NewRootSHA256, ports)
			}
		case "snapshot":
		case "preview":
			err = runtime.Preview(ctx)
		case "retry":
			err = runtime.Retry(ctx)
		case "configure":
			if request.Settings == nil {
				err = errors.New("complete reviewed settings required")
			} else {
				_, err = runtime.configureExpected(ctx, request.Revision, *request.Settings, request.ExpectedServer)
			}
		case "create-authority":
			var info AuthorityInfo
			info, err = runtime.CreateAuthority(ctx, request.Revision)
			if err == nil {
				response.Authority = &info
			}
		case "adopt-authority":
			var info AuthorityInfo
			info, err = runtime.AdoptAuthority(ctx, request.Revision, request.RootSHA256)
			if err == nil {
				response.Authority = &info
			}
		case "trust-status", "trust-install", "trust-remove":
			err = runtime.WindowsTrust(ctx, request.Operation, request.Revision, request.RootSHA256)
		case "stop-process":
			if request.ExpectedServer != nil && runtime.checkExpectedServer(request.ExpectedServer) != nil {
				err = errRestricted
			} else {
				response.shutdown = true
			}
		default:
			err = errors.New("unsupported local-services operation")
		}
	}
	if err == nil && runtime.opts.Restriction != nil && (request.Operation == "snapshot" || request.Operation == "configure") {
		fingerprint, fingerprintErr := runtime.request(ctx, runtimeCommand{kind: "restricted-fingerprint"})
		if fingerprintErr == nil {
			response.RestrictedStateSHA256 = fingerprint.restrictedSHA
		} else if request.Operation == "configure" {
			err = fingerprintErr
		}
	}
	if err != nil {
		response.Error = err.Error()
		if len(response.Error) > 4096 {
			response.Error = response.Error[:4096]
		}
	}
	state, stateErr := runtime.Snapshot()
	if stateErr != nil {
		response.Error = "local-services snapshot is unavailable"
	} else {
		response.State = &state
	}
	return response
}
func encodeServiceResponse(response ServiceResponse) []byte {
	data, err := json.Marshal(response)
	if err != nil || len(data) > ipcResponseBudget {
		return []byte(`{"version":1,"error":"local-services snapshot exceeds its response budget"}`)
	}
	return data
}
