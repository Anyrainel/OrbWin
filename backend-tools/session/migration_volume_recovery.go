package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

func openMigrationVolumeJob(target, token string) (*migrationVolumeJob, error) {
	if !validTransferToken(token) {
		return nil, errors.New("select an exact source maintenance receipt")
	}
	directory, err := migrationVolumeDirectory()
	if err != nil {
		return nil, err
	}
	lease, err := transientLease(filepath.Join(directory, token+".lock"), false)
	if err != nil {
		return nil, err
	}
	filename := filepath.Join(directory, token+".jsonl")
	before, err := os.Lstat(filename)
	if err != nil {
		lease.Close()
		return nil, err
	}
	if !before.Mode().IsRegular() {
		lease.Close()
		return nil, errors.New("maintenance receipt must be a regular file")
	}
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		lease.Close()
		return nil, err
	}
	fail := func(err error) (*migrationVolumeJob, error) { file.Close(); lease.Close(); return nil, err }
	after, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if !os.SameFile(before, after) {
		return fail(errors.New("maintenance receipt changed while opening"))
	}
	record, err := readMigrationVolumeRecord(file)
	if err != nil {
		return fail(err)
	}
	if record.Target != target || record.Token != token {
		return fail(errors.New("source maintenance receipt belongs to a different owned target"))
	}
	return &migrationVolumeJob{file: file, lease: lease, record: record}, nil
}
func recoverDesktopMigrationVolume(output io.Writer, target, token string) (resultErr error) {
	// The registered target ID is bound by the host receipt. Source-tool cleanup
	// must remain available when the owned engine is stopped or unavailable; it
	// verifies the distinct source before every mutation and never contacts the
	// target daemon or starts either engine.
	job, err := openMigrationVolumeJob(target, token)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, job.close()) }()
	scope := &migrationVolumeScope{token: job.record.Token, asset: job.record.AssetSHA, helper: job.record.helperName()}
	ctx, cancelSource := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancelSource()
	cli, _, closeSource, err := openMigrationVolumeSource(ctx, scope, job.record.Source)
	if err != nil {
		return err
	}
	defer closeSource()
	if err := cleanupMigrationVolume(ctx, cli, scope, job); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"toolsCleaned": true, "record": job.record, "message": "Only exact owned source tools were considered. Export was not resumed, source data and host packages were not removed. A missing export acknowledgement does not prove the destination is absent; inspect it before retrying."})
}
