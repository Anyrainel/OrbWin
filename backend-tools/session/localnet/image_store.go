package localnet

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

type ToolsImage struct {
	ID       string `json:"id"`
	AssetSHA string `json:"assetSha"`
	Token    string `json:"token"`
}
type toolsImageReceipt struct {
	Version  int    `json:"version"`
	Daemon   string `json:"daemon"`
	AssetSHA string `json:"assetSha"`
	Token    string `json:"token"`
	ID       string `json:"-"`
}

func (ledger *NamespaceLedger) imagePath(asset string) string {
	return filepath.Join(ledger.directory, "tools-image-"+asset+".receipt")
}

func (ledger *NamespaceLedger) committedImageReceipt(ctx context.Context, asset string) (out toolsImageReceipt, resultErr error) {
	if err := ledger.lockState(ctx); err != nil {
		return out, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return out, errors.New("exclusive image receipt owner required")
	}
	if ledger.stateStore != nil && ledger.pendingImport != nil {
		return out, errors.New("same native issuer has an unconfirmed image import; no lookup or replay can retire it")
	}
	return ledger.readImageReceipt(asset)
}
func (ledger *NamespaceLedger) verifyDaemon(ctx context.Context, cli *client.Client) error {
	if cli == nil {
		return errors.New("owned daemon client required")
	}
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != ledger.daemon || info.Info.OSType != "linux" {
		return errors.New("service image endpoint differs from the registered owned Linux daemon")
	}
	return nil
}
func (ledger *NamespaceLedger) readImageReceipt(asset string) (toolsImageReceipt, error) {
	var receipt toolsImageReceipt
	if !fullID(asset) {
		return receipt, errors.New("exact service asset digest required")
	}
	path := ledger.imagePath(asset)
	data, err := ledger.readState(path, 4096)
	if err != nil {
		return receipt, err
	}
	if len(data) > 4096 || len(data) == 0 || data[len(data)-1] != '\n' {
		return receipt, errors.New("partial service image receipt; retain for review")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) < 1 || len(lines) > 2 {
		return receipt, errors.New("unsupported service image receipt length")
	}
	if err := strictJSON(lines[0], &receipt); err != nil {
		return receipt, err
	}
	token, err := hex.DecodeString(receipt.Token)
	if receipt.Version != 1 || receipt.Daemon != ledger.daemon || receipt.AssetSHA != asset || err != nil || len(token) != 16 || receipt.Token != strings.ToLower(receipt.Token) {
		return receipt, errors.New("service image receipt identity/version differs")
	}
	if len(lines) == 2 {
		var accepted acceptedNamespace
		if err := strictJSON(lines[1], &accepted); err != nil {
			return receipt, err
		}
		if !strings.HasPrefix(accepted.ID, "sha256:") || !fullID(strings.TrimPrefix(accepted.ID, "sha256:")) {
			return receipt, errors.New("invalid accepted service image identity")
		}
		receipt.ID = accepted.ID
	}
	return receipt, nil
}
func verifyToolsImage(ctx context.Context, cli *client.Client, image ToolsImage) error {
	if !strings.HasPrefix(image.ID, "sha256:") || !fullID(strings.TrimPrefix(image.ID, "sha256:")) {
		return errors.New("exact service image ID required")
	}
	result, err := cli.ImageInspect(ctx, image.ID)
	if err != nil {
		return err
	}
	object := result.InspectResponse
	if object.ID != image.ID || object.Os != "linux" || object.Architecture != "amd64" || object.Config == nil || object.Config.Labels["dev.orbwin.component"] != "local-services-tools" || object.Config.Labels["dev.orbwin.source-sha256"] != image.AssetSHA || object.Config.Labels["dev.orbwin.import-token"] != image.Token {
		return errors.New("service image identity/provenance differs from its receipt")
	}
	return nil
}
func (ledger *NamespaceLedger) acceptImage(receipt toolsImageReceipt, id string) error {
	before, err := ledger.readState(ledger.imagePath(receipt.AssetSHA), 4096)
	if err != nil {
		return err
	}
	data, err := json.Marshal(acceptedNamespace{ID: id})
	if err != nil {
		return err
	}
	return ledger.appendState(ledger.imagePath(receipt.AssetSHA), before, append(data, '\n'))
}
func (ledger *NamespaceLedger) resolvePendingImage(ctx context.Context, cli *client.Client, receipt toolsImageReceipt) (ToolsImage, error) {
	listed, err := cli.ImageList(ctx, client.ImageListOptions{All: true, Filters: client.Filters{"label": {"dev.orbwin.component=local-services-tools": true, "dev.orbwin.source-sha256=" + receipt.AssetSHA: true, "dev.orbwin.import-token=" + receipt.Token: true}}})
	if err != nil {
		return ToolsImage{}, err
	}
	if len(listed.Items) != 1 {
		return ToolsImage{}, errors.New("service image import acceptance remains unknown or ambiguous; no import retried")
	}
	image := ToolsImage{ID: listed.Items[0].ID, AssetSHA: receipt.AssetSHA, Token: receipt.Token}
	if err := verifyToolsImage(ctx, cli, image); err != nil {
		return ToolsImage{}, err
	}
	if err := ledger.acceptImage(receipt, image.ID); err != nil {
		return ToolsImage{}, err
	}
	return image, nil
}

// Requires the exclusive controller owner. The flushed import token precedes
// ImageImport. Uncertain requests are resolved by narrow ownership lookup, never
// retried or pruned. Accepted IDs and reuse must match immutable image metadata.
func (ledger *NamespaceLedger) PrepareToolsImage(ctx context.Context, cli *client.Client, binary []byte, asset string) (out ToolsImage, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ledger.lockState(ctx); err != nil {
		return ToolsImage{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return ToolsImage{}, errors.New("exclusive local-services owner required")
	}
	if ledger.stateStore != nil && ledger.pendingImport != nil {
		return ToolsImage{}, errors.New("same native issuer has an unconfirmed image import; no lookup or replay can retire it")
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return ToolsImage{}, err
	}
	if err := validateToolsAsset(binary, asset, hex.EncodeToString(token[:])); err != nil {
		return ToolsImage{}, err
	}
	if err := ledger.verifyDaemon(ctx, cli); err != nil {
		return ToolsImage{}, err
	}
	receipt, err := ledger.readImageReceipt(asset)
	if err == nil {
		if receipt.ID == "" {
			if ledger.stateStore != nil {
				copy := receipt
				ledger.pendingImport = &copy
				return ToolsImage{}, errors.New("previous SDK image import lacks actual daemon-generation retirement; receipt lookup is not authority")
			}
			return ledger.resolvePendingImage(ctx, cli, receipt)
		}
		image := ToolsImage{ID: receipt.ID, AssetSHA: asset, Token: receipt.Token}
		if err := verifyToolsImage(ctx, cli, image); err != nil {
			return ToolsImage{}, fmt.Errorf("recorded service image unavailable; review explicit repair without replacing its receipt: %w", err)
		}
		return image, nil
	}
	if !os.IsNotExist(err) {
		return ToolsImage{}, err
	}
	receipt = toolsImageReceipt{Version: 1, Daemon: ledger.daemon, AssetSHA: asset, Token: hex.EncodeToString(token[:])}
	data, err := json.Marshal(receipt)
	if err != nil {
		return ToolsImage{}, err
	}
	if err = ledger.createState(ledger.imagePath(asset), append(data, '\n')); err != nil {
		return ToolsImage{}, err
	}
	copy := receipt
	ledger.pendingImport = &copy // BEFORE every possible dispatch, not an error flag reconstructed afterward.
	id, err := ImportToolsImage(ctx, cli, binary, asset, receipt.Token)
	if err != nil {
		return ToolsImage{}, fmt.Errorf("service image import uncertain/refused; ownership receipt retained: %w", err)
	}
	if err := ledger.acceptImage(receipt, id); err != nil {
		return ToolsImage{}, err
	}
	ledger.pendingImport = nil // Same original request positively completed and accepted.
	return ToolsImage{ID: id, AssetSHA: asset, Token: receipt.Token}, nil
}

// Explicit repair archives evidence, not an image. A known image must be absent
// before its active receipt can be retired. Unknown acceptance may finish late;
// its token remains in the archived receipt for later recovery. Nothing prunes,
// retags or deletes daemon images, workload data or an existing archive.
func (ledger *NamespaceLedger) ArchiveToolsImageReceipt(ctx context.Context, cli *client.Client, asset, expectedToken string) (out string, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ledger.lockState(ctx); err != nil {
		return "", err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return "", errors.New("exclusive local-services owner required")
	}
	if err := ledger.verifyDaemon(ctx, cli); err != nil {
		return "", err
	}
	receipt, err := ledger.readImageReceipt(asset)
	if err != nil {
		return "", err
	}
	if receipt.Token != expectedToken {
		return "", errors.New("service image receipt changed since repair review")
	}
	if receipt.ID != "" {
		_, err := cli.ImageInspect(ctx, receipt.ID)
		if !cerrdefs.IsNotFound(err) {
			if err != nil {
				return "", err
			}
			return "", errors.New("recorded service image still exists; reuse it instead of retiring its receipt")
		}
	}
	source := ledger.imagePath(asset)
	destination := filepath.Join(ledger.directory, "tools-image-"+asset+"-"+receipt.Token+".archived-receipt")
	if ledger.stateStore != nil {
		before, err := ledger.readState(source, 4096)
		if err != nil {
			return "", err
		}
		existing, err := ledger.readState(destination, 4096)
		if err == nil {
			if !bytes.Equal(existing, before) {
				return "", errors.New("managed image archive conflicts; retain original receipt")
			}
		} else if !os.IsNotExist(err) {
			return "", err
		} else if err = ledger.createState(destination, before); err != nil {
			return "", err
		}
		return destination, ledger.deleteState(source, before)
	}
	if err := os.Link(source, destination); err != nil {
		original, firstErr := os.Lstat(source)
		archived, secondErr := os.Lstat(destination)
		if firstErr != nil || secondErr != nil || !original.Mode().IsRegular() || !archived.Mode().IsRegular() || !os.SameFile(original, archived) {
			return "", errors.New("service image archive publication failed or conflicts; original receipt retained")
		}
	}
	if err := os.Remove(source); err != nil {
		return destination, err
	}
	return destination, nil
}
