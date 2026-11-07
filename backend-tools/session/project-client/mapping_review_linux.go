//go:build linux

package main

// This is a bounded, read-only use of the existing connected project. A held
// root/file descriptor and before/after metadata checks refuse observed drift;
// Linux writers are not excluded and later/ABA changes are not ruled out.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"reflect"
	"regexp"
	"syscall"
)

func sameMappingState(a, b os.FileInfo) bool {
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && a.Mode().IsRegular() && b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && x.Mtim == y.Mtim && x.Ctim == y.Ctim
}
func inspectProjectMapping(ctx context.Context, root *os.Root, signature string, review mappingReview, check func() error) error {
	if !mappingFilename(review.File) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(review.SHA256) {
		return errors.New("invalid saved mapping review")
	}
	expected, err := checkedMapping(review.Contents)
	if err != nil {
		return err
	}
	current, err := rootIdentity(root)
	if err != nil || current != signature {
		return errors.New("opened project root changed")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = check(); err != nil {
		return err
	}
	file, err := root.OpenFile(review.File, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || before.Size() > 256<<10 {
		return errors.New("saved mapping must be a regular file within 256 KiB")
	}
	bytes, err := io.ReadAll(io.LimitReader(file, (256<<10)+1))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(bytes)
	if len(bytes) > 256<<10 || hex.EncodeToString(digest[:]) != review.SHA256 {
		return errors.New("saved mapping bytes changed; review and save again")
	}
	actual, err := checkedMapping(string(bytes))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return errors.New("saved mapping semantics differ from reviewed source mounts/destinations")
	}
	if err = check(); err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	named, err := root.Lstat(review.File)
	if err != nil {
		return err
	}
	if !sameMappingState(before, after) || !sameMappingState(after, named) {
		return errors.New("saved mapping changed while checking")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return nil
}
func (request workflowRequest) verifyMapping(ctx context.Context) error {
	if request.MappingReview == nil {
		return nil
	}
	if request.Kind != "compose" || (request.Action != "validate" && request.Action != "up") || request.ReviewedDaemon == "" || request.ReviewedDaemon != request.Daemon {
		return errors.New("mapping review requires a reviewed Compose target and Validate/Up")
	}
	expected := request.Shared + "/" + request.MappingReview.File
	files := append([]string{request.File}, request.Overrides...)
	count := 0
	for index, file := range files {
		mapped, err := request.mappedPath(file, pathFile)
		if err != nil {
			return err
		}
		if mapped == expected {
			count++
			if index == 0 || index != len(files)-1 {
				return errors.New("saved mapping must be the unique final override")
			}
		}
	}
	if count != 1 {
		return errors.New("saved mapping must be the unique final override")
	}
	root, err := os.OpenRoot(request.Source)
	if err != nil {
		return err
	}
	defer root.Close()
	return inspectProjectMapping(ctx, root, request.Signature, *request.MappingReview, request.verifyProject)
}
