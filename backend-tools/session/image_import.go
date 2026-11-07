package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/moby/moby/client"
)

func guardImageImportTag(ctx context.Context, cli *client.Client, choice imageImportChoice) error {
	actual, err := imageImportCurrentTag(ctx, cli, choice.Reference)
	if err != nil {
		return err
	}
	if actual != choice.ExpectedID {
		return fmt.Errorf("destination tag %s changed since review; retain the archive and review again", choice.Reference)
	}
	return nil
}
func loadIsolatedImageArchive(ctx context.Context, cli *client.Client, source io.ReaderAt, index imageArchiveIndex, staging []string) error {
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := writeImageImportArchive(ctx, writer, source, index, staging)
		_ = writer.CloseWithError(err)
		done <- err
	}()
	loaded, err := cli.ImageLoad(ctx, reader, client.ImageLoadWithQuiet(true))
	if err == nil {
		limited := &io.LimitedReader{R: loaded, N: (4 << 20) + 1}
		decoder := json.NewDecoder(limited)
		for {
			var event struct {
				Stream      string `json:"stream"`
				Error       string `json:"error"`
				ErrorDetail *struct {
					Message string `json:"message"`
				} `json:"errorDetail"`
			}
			readErr := decoder.Decode(&event)
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				err = readErr
				break
			}
			if event.Error != "" {
				err = errors.New(event.Error)
				break
			}
			if event.ErrorDetail != nil && event.ErrorDetail.Message != "" {
				err = errors.New(event.ErrorDetail.Message)
				break
			}
			if len(event.Stream) > 8192 {
				event.Stream = event.Stream[:8192]
			}
			if event.Stream != "" {
				fmt.Fprint(os.Stderr, event.Stream)
			}
		}
		if limited.N <= 0 {
			err = errors.Join(err, errors.New("image Load response exceeded its budget"))
		}
		err = errors.Join(err, loaded.Close())
	}
	// Stop a transport-side producer even if the daemon rejected the request
	// before consuming it. Wait before the caller releases the locked archive.
	_ = reader.CloseWithError(errors.New("image Load request finished"))
	return errors.Join(err, <-done)
}
func verifyStagedImage(ctx context.Context, cli *client.Client, source io.ReaderAt, index imageArchiveIndex, stage imageImportStage) (id string, resultErr error) {
	position := -1
	for i, image := range index.Images {
		if image.ID == stage.ConfigID {
			position = i
			break
		}
	}
	if position < 0 {
		return "", errors.New("staged config was not in the reviewed archive")
	}
	config, err := imageArchiveMetadata(source, index.Entries, index.Manifest[position].Config, imageArchiveConfigLimit)
	if err != nil {
		return "", err
	}
	var parsed struct {
		RootFS struct {
			DiffIDs []string `json:"diff_ids"`
		} `json:"rootfs"`
	}
	if err := json.Unmarshal(config, &parsed); err != nil {
		return "", err
	}
	before, err := cli.ImageInspect(ctx, stage.Reference)
	if err != nil {
		return "", err
	}
	if !imageIDValid(before.ID) {
		return "", errors.New("staged image has an unsupported identity")
	}
	stream, err := cli.ImageSave(ctx, []string{stage.Reference})
	if err != nil {
		return "", err
	}
	err = verifyImageImportStream(stream, stage.ConfigID, parsed.RootFS.DiffIDs)
	err = errors.Join(err, stream.Close())
	if err != nil {
		return "", err
	}
	after, err := cli.ImageInspect(ctx, stage.Reference)
	if err != nil {
		return "", err
	}
	if after.ID != before.ID {
		return "", errors.New("isolated image reference changed during verification")
	}
	return before.ID, nil
}
func importImages(input io.Reader, output io.Writer, daemon string) (resultErr error) {
	data, err := io.ReadAll(io.LimitReader(input, (512<<10)+1))
	if err != nil {
		return err
	}
	if len(data) > 512<<10 {
		return errors.New("reviewed image import request exceeds its size budget")
	}
	var request imageImportRequest
	if err := strictCloneJSON(data, &request); err != nil {
		return err
	}
	if !digestValid(request.SHA256) {
		return errors.New("invalid or oversized reviewed image import request")
	}
	file, index, err := imageImportOpen(request.Source)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	if index.SHA256 != request.SHA256 {
		return errors.New("image archive differs from the reviewed checksum; no load was sent")
	}
	request.Tags, err = imageImportChoices(index, request.Tags)
	if err != nil {
		return err
	}
	cli, ctx, cancel, err := transferClient(daemon)
	if err != nil {
		return err
	}
	defer cancel()
	defer func() { resultErr = errors.Join(resultErr, cli.Close()) }()
	for _, choice := range request.Tags {
		if choice.Apply {
			if err := guardImageImportTag(ctx, cli, choice); err != nil {
				return err
			}
		}
	}
	if err := imageImportGuestSpace(ctx, cli, index, daemon); err != nil {
		return err
	}
	job, err := beginImageImport(daemon, request, index)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, job.close()) }()
	fmt.Fprintln(os.Stderr, "Image import ownership/recovery receipt:", job.file.Name())
	staging := []string{}
	for _, image := range job.record.Images {
		actual, err := imageImportCurrentTag(ctx, cli, image.Reference)
		if err != nil {
			return err
		}
		if actual != "" {
			return errors.New("random isolated image reference already exists; no load was sent")
		}
		staging = append(staging, image.Reference)
	}
	for _, reference := range job.record.PreviousReferences {
		actual, err := imageImportCurrentTag(ctx, cli, reference)
		if err != nil {
			return err
		}
		if actual != "" {
			return errors.New("random previous-content reference already exists; no load was sent")
		}
	}
	if err := job.append(imageImportEvent{Phase: "loading"}); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Loading isolated image references; no original tags are written during Load")
	if err := loadIsolatedImageArchive(ctx, cli, file, index, staging); err != nil {
		return fmt.Errorf("isolated image load unconfirmed; recovery references and receipt retained: %w", err)
	}
	for _, stage := range job.record.Images {
		fmt.Fprintln(os.Stderr, "Verifying staged image content:", stage.Reference)
		id, err := verifyStagedImage(ctx, cli, file, index, stage)
		if err != nil {
			return fmt.Errorf("staged image verification failed; no original tags were written: %w", err)
		}
		if err := job.append(imageImportEvent{Phase: "verified", ConfigID: stage.ConfigID, ImageID: id}); err != nil {
			return err
		}
	}
	for _, choice := range request.Tags {
		if choice.Apply {
			if err := guardImageImportTag(ctx, cli, choice); err != nil {
				return err
			}
		}
	}
	for _, choice := range request.Tags {
		if !choice.Apply || choice.ExpectedID == "" {
			continue
		}
		if err := guardImageImportTag(ctx, cli, choice); err != nil {
			return err
		}
		if err := job.append(imageImportEvent{Phase: "previous-intent", Reference: choice.Reference, ImageID: choice.ExpectedID}); err != nil {
			return err
		}
		backup := job.record.PreviousReferences[choice.Reference]
		if actual, err := imageImportCurrentTag(ctx, cli, backup); err != nil {
			return err
		} else if actual != "" {
			return errors.New("isolated previous-content reference is no longer absent")
		}
		if _, err := cli.ImageTag(ctx, client.ImageTagOptions{Source: choice.ExpectedID, Target: backup}); err != nil {
			return err
		}
		actual, err := imageImportCurrentTag(ctx, cli, backup)
		if err != nil {
			return err
		}
		if actual != choice.ExpectedID {
			return errors.New("previous image retention could not be confirmed; no original tags were written")
		}
		if err := job.append(imageImportEvent{Phase: "previous-retained", Reference: choice.Reference, ImageID: choice.ExpectedID}); err != nil {
			return err
		}
	}
	if err := job.append(imageImportEvent{Phase: "tagging"}); err != nil {
		return err
	}
	for _, choice := range request.Tags {
		if !choice.Apply {
			continue
		}
		if err := guardImageImportTag(ctx, cli, choice); err != nil {
			return err
		}
		id := job.record.Verified[choice.ConfigID]
		if err := job.append(imageImportEvent{Phase: "tag-intent", ConfigID: choice.ConfigID, ImageID: id, Reference: choice.Reference}); err != nil {
			return err
		}
		if err := guardImageImportTag(ctx, cli, choice); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Applying explicitly reviewed image tag:", choice.Reference)
		if _, err := cli.ImageTag(ctx, client.ImageTagOptions{Source: id, Target: choice.Reference}); err != nil {
			return fmt.Errorf("tag write unconfirmed; inspect the retained receipt before retrying: %w", err)
		}
		actual, err := imageImportCurrentTag(ctx, cli, choice.Reference)
		if err != nil {
			return err
		}
		if actual != id {
			return errors.New("image tag changed after application; previous content and receipt retained")
		}
		if err := job.append(imageImportEvent{Phase: "tagged", ConfigID: choice.ConfigID, ImageID: id, Reference: choice.Reference}); err != nil {
			return err
		}
	}
	// Re-observe every applied reference, not just the last successful API call.
	for _, choice := range request.Tags {
		if choice.Apply {
			actual, err := imageImportCurrentTag(ctx, cli, choice.Reference)
			if err != nil {
				return err
			}
			if actual != job.record.Verified[choice.ConfigID] {
				return errors.New("an applied image tag changed before completion; import remains unconfirmed")
			}
		}
	}
	if err := job.append(imageImportEvent{Phase: "complete"}); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"complete": true, "transaction": job.record.Transaction, "receipt": job.file.Name(), "sha256": index.SHA256, "images": job.record.Images, "verified": job.record.Verified, "tagged": job.record.Tagged, "choices": request.Tags, "previousReferences": job.record.PreviousReferences, "previous": job.record.Previous, "message": "Imported configs/layers were verified. Isolated recovery references remain in image inventory. Skipped original tags were not changed; previous replacement content has retained references."})
}
