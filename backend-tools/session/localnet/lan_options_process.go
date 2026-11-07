package localnet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

type boundedLANOutput struct {
	data  bytes.Buffer
	limit int
}

func (w *boundedLANOutput) Write(b []byte) (int, error) {
	if len(b) > w.limit-w.data.Len() {
		return 0, errors.New("LAN observation output budget exceeded")
	}
	return w.data.Write(b)
}

// One exact, read-only SDK child bounds synchronous IP Helper/NLM calls. The
// caller retains its single query slot until Wait returns, including after an
// RPC cancellation. No target executable/path/arguments come from the UI.
func readNativeLANChild(ctx context.Context, expectedSHA string) (NativeLANObservation, error) {
	var empty NativeLANObservation
	if !fullID(expectedSHA) {
		return empty, errors.New("LAN diagnostic SDK identity unavailable")
	}
	exe, err := os.Executable()
	if err != nil {
		return empty, err
	}
	file, err := os.Open(exe)
	if err != nil {
		return empty, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > 256<<20 {
		return empty, errors.New("bounded LAN diagnostic image unavailable")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, (256<<20)+1))
	if err != nil || n != before.Size() || hex.EncodeToString(hash.Sum(nil)) != expectedSHA {
		return empty, errors.New("LAN diagnostic SDK image differs")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, exe, "lan-network-observation")
	if err = prepareReadOnlyLANChild(command); err != nil {
		return empty, err
	}
	command.WaitDelay = time.Second
	var output = boundedLANOutput{limit: (256 + 64) << 10}
	command.Stdout, command.Stderr = &output, io.Discard
	if err = command.Run(); err != nil {
		return empty, errors.New("bounded LAN native observation unavailable")
	}
	after, err := os.Stat(exe)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		return empty, errors.New("LAN diagnostic SDK changed during observation")
	}
	if err = strictJSON(output.data.Bytes(), &empty); err != nil {
		return NativeLANObservation{}, errors.New("LAN diagnostic observation schema differs")
	}
	return empty, nil
}
