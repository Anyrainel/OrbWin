package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// This step only exports the reviewed source image to a new portable archive.
// Import is the separately reviewed owned-engine transaction, including tag
// collision consent and readback verification. No source tags are changed.
func exportDesktopMigrationImage(output io.Writer, target, source, image, reference, destination string) error {
	if source == "" || source == target {
		return errors.New("distinct reviewed source and owned target daemon IDs required")
	}
	if err := validateImageExport(image, reference, destination); err != nil {
		return err
	}
	owned, _, cancel, err := transferClient(target)
	if err != nil {
		return err
	}
	defer cancel()
	defer owned.Close()
	cli, closeSource, err := newDesktopMigrationClient()
	if err != nil {
		return err
	}
	defer closeSource()
	ctx, cancelSource := context.WithTimeout(context.Background(), 1800*time.Second)
	defer cancelSource()
	var identity migrationSourceIdentity
	guard := func() error {
		var err error
		identity, err = verifyDesktopMigrationSource(ctx, cli, target, source)
		return err
	}
	if err := guard(); err != nil {
		return err
	}
	return journalMigrationImage(ctx, output, identity, target, image, reference, destination, func(w io.Writer) error {
		return exportClientImage(ctx, cli, w, image, reference, destination, guard)
	})
}

// The exporter owns archive publication; this owner only acknowledges its
// verified result. Intent is durable before ImageSave. A failed or interrupted
// export retains its intent and any archive without authorizing automatic retry.
func journalMigrationImage(ctx context.Context, output io.Writer, source migrationSourceIdentity, target, image, reference, destination string, export func(io.Writer) error) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	token, err := transferToken()
	if err != nil {
		return err
	}
	job, err := beginMigrationImage(migrationImageIntent{Version: 1, Transaction: token, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: source, Target: target, Image: image, Reference: reference, Destination: destination})
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, job.close()) }()
	uncertain := func(err error) error {
		return fmt.Errorf("source image export was not acknowledged; preserve archive/partial files and receipt %s: %w", job.file.Name(), err)
	}
	var encoded bytes.Buffer
	if err := export(&encoded); err != nil {
		return uncertain(err)
	}
	if err := ctx.Err(); err != nil {
		return uncertain(err)
	}
	var result struct {
		Complete  bool                `json:"complete"`
		Path      string              `json:"path"`
		Image     string              `json:"image"`
		Reference string              `json:"reference"`
		SHA256    string              `json:"sha256"`
		Bytes     int64               `json:"bytes"`
		Images    []imageArchiveImage `json:"images"`
	}
	if err := strictCloneJSON(encoded.Bytes(), &result); err != nil {
		return uncertain(err)
	}
	if !result.Complete || result.Path != destination || result.Image != image || result.Reference != reference {
		return uncertain(errors.New("image export result differs from its durable intent"))
	}
	ack := migrationImageAcknowledgement{Phase: "exported", SHA256: result.SHA256, Bytes: result.Bytes}
	for _, item := range result.Images {
		ack.ConfigIDs = append(ack.ConfigIDs, item.ID)
	}
	if err := job.acknowledge(ack); err != nil {
		return uncertain(err)
	}
	var response map[string]any
	if err := json.Unmarshal(encoded.Bytes(), &response); err != nil {
		return err
	}
	response["source"], response["target"] = source, target
	response["transaction"], response["receipt"] = token, job.file.Name()
	return json.NewEncoder(output).Encode(response)
}
