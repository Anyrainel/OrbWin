package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
	"orbwin.local/session/localnet"
)

func serviceHome(raw string) (string, error) {
	volume := filepath.VolumeName(raw)
	if !filepath.IsAbs(raw) || len(volume) != 2 || volume[1] != ':' || strings.ContainsAny(raw, "\x00\r\n") {
		return "", errors.New("registered local drive home required")
	}
	clean := filepath.Clean(raw)
	canonical, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(canonical, clean) {
		return "", errors.New("registered home must not traverse reparse aliases")
	}
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		return "", errors.New("registered home directory unavailable")
	}
	return clean, nil
}
func runServiceArguments(args []string) (handled bool, resultErr error) {
	if len(args) == 0 || (args[0] != "local-services-serve" && args[0] != "local-services-call") {
		return false, nil
	}
	var restriction *localnet.RestrictiveStartup
	if args[0] == "local-services-serve" {
		cleaned, mode, fingerprint, parseErr := parseServiceRestriction(args)
		if parseErr != nil {
			return true, parseErr
		}
		args = cleaned
		if mode != 0 {
			restriction = &localnet.RestrictiveStartup{RequireNew: mode == 1, DisabledStateSHA256: fingerprint}
		}
	}
	if (args[0] == "local-services-serve" && len(args) != 6 && len(args) != 8) || (args[0] == "local-services-call" && len(args) != 4) {
		return true, errors.New("invalid local-services arguments")
	}
	home, err := serviceHome(args[1])
	if err != nil {
		return true, err
	}
	if args[0] == "local-services-call" {
		if len(args[3]) > 192<<10 {
			return true, errors.New("local-services request exceeds its input budget")
		}
		var data []byte
		if args[3] == "-" {
			data, err = io.ReadAll(io.LimitReader(os.Stdin, localnet.MaxServiceRequestBytes+1))
		} else {
			data, err = base64.StdEncoding.Strict().DecodeString(args[3])
		}
		if err != nil {
			return true, err
		}
		if len(data) > localnet.MaxServiceRequestBytes {
			return true, errors.New("local-services request exceeds its frame budget")
		}
		var request localnet.ServiceRequest
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&request); err != nil {
			return true, err
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return true, errors.New("unexpected request suffix")
		}
		ctx, cancel := context.WithTimeout(context.Background(), localnet.ServiceOperationTimeout(request)+5*time.Second)
		defer cancel()
		response, err := localnet.CallServiceIPC(ctx, home, args[2], request)
		if err != nil {
			return true, err
		}
		return true, json.NewEncoder(os.Stdout).Encode(response)
	}
	owners := &serviceStartupOwners{}
	defer func() {
		bodyPanic := recover() // FIRST, before any resource close on this producer.
		resultErr = errors.Join(resultErr, finishServiceStartup(bodyPanic,
			func() error {
				if owners.root != nil {
					return owners.root.Close()
				}
				return nil
			},
			func() error {
				if owners.client != nil {
					return owners.client.Close()
				}
				return nil
			},
			func() error {
				if owners.self != nil {
					return owners.self.Close()
				}
				return nil
			},
			func() { retainServiceStartup(owners) }))
	}()

	startup, startupDone := context.WithTimeout(context.Background(), 5*time.Second)
	selfLease, err := sdkself.Hold(startup)
	owners.self = selfLease
	startupDone()

	if err != nil {
		return true, errors.New("local-services retained signed SDK unavailable")
	}
	// The sole cleanup defer runs AFTER runtime.Close and its positive Wait. UI
	// exit cannot release this process's independent image/bundle leases.
	selfIdentity, err := selfLease.Identity()
	if err != nil {
		return true, err
	}
	assetPath, err := filepath.Abs(args[4])
	if err != nil {
		return true, err
	}
	info, err := os.Lstat(assetPath)
	if err != nil {
		return true, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return true, errors.New("verified static services asset required")
	}
	file, err := os.Open(assetPath)
	if err != nil {
		return true, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		file.Close()
		return true, errors.New("service asset changed while opening")
	}
	asset, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	closed := file.Close()
	if err != nil || closed != nil {
		return true, errors.Join(err, closed)
	}
	var trustAsset []byte
	var trustHash string
	if len(args) == 8 {
		trustAsset, err = readWSLTrustAsset(args[6])
		if err != nil {
			return true, err
		}
		trustHash = args[7]
	}
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return true, err
	}
	owners.client = cli
	stateStart, stateDone := context.WithTimeout(context.Background(), 5*time.Second)
	var stateRoot *sdkstate.Root
	if restriction != nil && restriction.RequireNew {
		stateRoot, err = sdkstate.OpenNewService(stateStart, args[2], args[3], home)
	} else {
		stateRoot, err = sdkstate.Open(stateStart, args[2], args[3], home)
	}
	owners.root = stateRoot
	stateDone()

	if err != nil {
		return true, err
	} // Legacy or damaged origin never becomes a fresh CA.
	if restriction != nil {
		checkCtx, checkDone := context.WithTimeout(context.Background(), 5*time.Second)
		err = localnet.CheckRestrictiveStartup(checkCtx, stateRoot, args[3], *restriction)
		checkDone()
		if err != nil {
			return true, err
		}
	}
	ledger, err := localnet.OpenManagedNamespaceLedger(stateRoot, args[3])
	if err != nil {
		return true, err
	}
	owners.ledger = ledger
	runtime, err := localnet.NewServiceRuntime(localnet.RuntimeOptions{Restriction: restriction, SelfLease: selfLease, Client: cli, Ledger: ledger, Installation: args[2], ToolsAsset: asset, ToolsSHA256: args[5], BinarySHA256: selfIdentity.ImageSHA256, SelfIdentity: &selfIdentity, WSLTrustAsset: trustAsset, WSLTrustSHA256: trustHash, NativeLAN: true})
	owners.runtime = runtime
	if err != nil {
		if runtime != nil {
			fmt.Fprintln(os.Stderr, "OrbWin local-services construction retained for inspection:", err)
			return true, errors.Join(err, runtime.Wait(context.Background()))
		}
		if closeErr := ledger.Close(); closeErr != nil {
			fmt.Fprintln(os.Stderr, "OrbWin local-services ledger retained for inspection:", errors.Join(err, closeErr))
			for {
				time.Sleep(time.Minute)
				goruntime.KeepAlive(ledger)
			}
		}
		return true, err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err = localnet.ServeServiceIPC(ctx, runtime, home, args[2])
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	cleanup, done := context.WithTimeout(context.Background(), 40*time.Second)
	closeErr := runtime.Close(cleanup)
	done()
	// A bounded RPC response may report cleanup-pending. Keep the SAME process
	// and exclusive owner alive for its existing join worker; do not retry Close
	// or release the ledger while a child/namespace outcome remains unknown.
	return true, errors.Join(err, closeErr, runtime.Wait(context.Background()))
}

func readWSLTrustAsset(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil, errors.New("bounded verified WSL trust asset required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		file.Close()
		return nil, errors.New("WSL trust asset changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	return data, errors.Join(err, file.Close())
}

func serviceExitCode(err error) int {
	if errors.Is(err, localnet.ErrServiceNotRunning) {
		return 126
	}
	if errors.Is(err, sdkstate.ErrLegacyState) {
		return 124
	}
	if errors.Is(err, sdkstate.ErrRecoveryRequired) {
		return 123
	}
	if errors.Is(err, sdkstate.ErrState) {
		return 122
	}
	return 125
}
