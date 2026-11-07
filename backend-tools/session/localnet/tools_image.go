package localnet

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ImportToolsImage creates only a scratch-root image from an already verified
// local static asset. No registry, Dockerfile command, tag replacement, host
// directory or selected workload data is read. The caller persists a unique
// import intent/token first; an uncertain import must not trigger broad pruning.
func validateToolsAsset(binary []byte, expectedSHA, token string) error {
	hash := sha256.Sum256(binary)
	identity, err := hex.DecodeString(token)
	if len(binary) < 20 || len(binary) > 16<<20 || string(binary[:4]) != "\x7fELF" || binary[4] != 2 || binary[5] != 1 || binary[18] != 62 || binary[19] != 0 || hex.EncodeToString(hash[:]) != expectedSHA || err != nil || len(identity) != 16 || token != strings.ToLower(token) {
		return errors.New("verified static Linux/amd64 service asset and import ownership token required")
	}
	return nil
}
func ImportToolsImage(ctx context.Context, cli *client.Client, binary []byte, expectedSHA, token string) (string, error) {
	return importStaticToolsImage(ctx, cli, binary, expectedSHA, token, "orbwin-services", "local-services-tools")
}

// ImportFileToolsImage shares the verified scratch asset import mechanism, but
// has its own executable and provenance component. Its caller owns a separate
// maintenance intent and must never adopt a local-services image/receipt.
func ImportFileToolsImage(ctx context.Context, cli *client.Client, binary []byte, expectedSHA, token string) (string, error) {
	return importStaticToolsImage(ctx, cli, binary, expectedSHA, token, "orbwin-fs", "file-maintenance-tools")
}

func importStaticToolsImage(ctx context.Context, cli *client.Client, binary []byte, expectedSHA, token, executable, component string) (string, error) {
	if err := validateToolsAsset(binary, expectedSHA, token); err != nil {
		return "", err
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	var err error
	for _, directory := range []string{"etc/", "run/", "run/orbwin/"} {
		if err = writer.WriteHeader(&tar.Header{Name: directory, Mode: 0755, Typeflag: tar.TypeDir}); err != nil {
			return "", err
		}
	}
	if err = writer.WriteHeader(&tar.Header{Name: executable, Mode: 0755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		return "", err
	}
	if _, err = writer.Write(binary); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	imported, err := cli.ImageImport(ctx, client.ImageImportSource{SourceName: "-", Source: &archive}, "", client.ImageImportOptions{Platform: ocispec.Platform{OS: "linux", Architecture: "amd64"}, Changes: []string{"LABEL dev.orbwin.component=" + component, "LABEL dev.orbwin.source-sha256=" + expectedSHA, "LABEL dev.orbwin.import-token=" + token}})
	if err != nil {
		return "", err
	}
	defer imported.Close()
	limited := &io.LimitedReader{R: imported, N: 65537}
	decoder := json.NewDecoder(limited)
	imageID := ""
	for {
		var message struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		err = decoder.Decode(&message)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if message.Error != "" {
			return "", fmt.Errorf("static tools import refused: %s", message.Error)
		}
		digest := strings.TrimPrefix(message.Status, "sha256:")
		raw, decodeErr := hex.DecodeString(digest)
		if strings.HasPrefix(message.Status, "sha256:") && decodeErr == nil && len(raw) == 32 {
			if imageID != "" && imageID != message.Status {
				return "", errors.New("tools import returned conflicting image identities; retain its import intent")
			}
			imageID = message.Status
		}
	}
	if limited.N <= 0 || imageID == "" {
		return "", errors.New("tools import did not return a bounded exact image identity; retain its import intent")
	}
	inspected, err := cli.ImageInspect(ctx, imageID)
	if err != nil {
		return "", err
	}
	object := inspected.InspectResponse
	if object.ID != imageID || object.Os != "linux" || object.Architecture != "amd64" || object.Config == nil || object.Config.Labels["dev.orbwin.component"] != component || object.Config.Labels["dev.orbwin.source-sha256"] != expectedSHA || object.Config.Labels["dev.orbwin.import-token"] != token {
		return "", errors.New("imported static tools image ownership differs; no container launched")
	}
	return imageID, nil
}
