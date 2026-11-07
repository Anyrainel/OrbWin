package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

func trustedFileHelper() ([]byte, error) {
	file, err := os.Open(os.Getenv("ORBWIN_FS_HELPER"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	binary, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(binary)
	if len(binary) > 16<<20 || hex.EncodeToString(hash[:]) != os.Getenv("ORBWIN_FS_HELPER_SHA256") {
		return nil, errors.New("guest file helper does not match the verified binary")
	}
	return binary, nil
}
func stageFileHelper(ctx context.Context, cli *client.Client, id string) error {
	binary, err := trustedFileHelper()
	if err != nil {
		return err
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	if err = writer.WriteHeader(&tar.Header{Name: "orbwin-fs", Mode: 0755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err = writer.Write(binary); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	_, err = cli.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: "/usr/local/bin", Content: &archive})
	return err
}
func auditVolumeAttributes(ctx context.Context, cli *client.Client, id string) error {
	return auditVolumeAttributesAt(ctx, cli, id, "/usr/local/bin/orbwin-fs")
}
func auditVolumeAttributesAt(ctx context.Context, cli *client.Client, id, executable string) error {
	return auditVolumeAttributesScoped(ctx, cli, id, executable, nil)
}
func auditVolumeAttributesScoped(ctx context.Context, cli *client.Client, id, executable string, acceptExec func(string) error) error {
	operation, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{Cmd: []string{executable, "check-volume"}, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return err
	}
	if acceptExec != nil {
		if err := acceptExec(operation.ID); err != nil {
			return err
		}
	}
	attached, err := cli.ExecAttach(ctx, operation.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attached.Close()
	if err = attached.Conn.SetDeadline(time.Now().Add(120 * time.Second)); err != nil {
		return err
	}
	stdout, stderr := &boundedBuffer{left: 8192}, &boundedBuffer{left: 8192}
	if _, err = stdcopy.StdCopy(stdout, stderr, attached.Reader); err != nil {
		return err
	}
	settled, err := cli.ExecInspect(ctx, operation.ID, client.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if settled.Running || settled.ExitCode != 0 {
		return errors.New("portable metadata audit refused: " + stderr.String())
	}
	return nil
}
