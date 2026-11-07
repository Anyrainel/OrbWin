package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/moby/moby/client"
)

type imageImportTagReview struct {
	Reference string   `json:"reference"`
	ConfigIDs []string `json:"configIds"`
	CurrentID string   `json:"currentId"`
}
type imageImportReview struct {
	Version int                    `json:"version"`
	Source  string                 `json:"source"`
	SHA256  string                 `json:"sha256"`
	Bytes   int64                  `json:"bytes"`
	Images  []imageArchiveImage    `json:"images"`
	Tags    []imageImportTagReview `json:"tags"`
}
type imageImportChoice struct {
	Reference  string `json:"reference"`
	ConfigID   string `json:"configId"`
	ExpectedID string `json:"expectedId"`
	Apply      bool   `json:"apply"`
	Replace    bool   `json:"replace"`
}
type imageImportRequest struct {
	Source string              `json:"source"`
	SHA256 string              `json:"sha256"`
	Tags   []imageImportChoice `json:"tags"`
}

func imageImportReference(value string) (string, error) {
	if len(value) > 512 || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("image archive tag exceeds its input budget")
	}
	parsed, err := reference.ParseNormalizedNamed(value)
	if err != nil {
		return "", err
	}
	if _, ok := parsed.(reference.NamedTagged); !ok {
		return "", errors.New("image import requires explicit tags")
	}
	if _, ok := parsed.(reference.Digested); ok {
		return "", errors.New("image import does not write digest references")
	}
	if strings.HasPrefix(parsed.Name(), "docker.io/orbwin-imports/") {
		return "", errors.New("orbwin-imports is reserved for isolated recovery references")
	}
	if len(parsed.String()) > 512 {
		return "", errors.New("normalized image tag exceeds its input budget")
	}
	return parsed.String(), nil
}
func imageImportTagPlan(index imageArchiveIndex) ([]imageImportTagReview, error) {
	groups := map[string]map[string]bool{}
	for _, image := range index.Images {
		for _, tag := range image.Tags {
			normalized, err := imageImportReference(tag)
			if err != nil {
				return nil, fmt.Errorf("unsupported archived tag %q: %w", tag, err)
			}
			if groups[normalized] == nil {
				groups[normalized] = map[string]bool{}
			}
			groups[normalized][image.ID] = true
			if len(groups) > 512 {
				return nil, errors.New("image import review exceeds its 512-tag budget")
			}
		}
	}
	result := []imageImportTagReview{}
	for tag, configs := range groups {
		ids := []string{}
		for id := range configs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		result = append(result, imageImportTagReview{Reference: tag, ConfigIDs: ids})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Reference < result[j].Reference })
	return result, nil
}
func imageImportChoices(index imageArchiveIndex, choices []imageImportChoice) ([]imageImportChoice, error) {
	plan, err := imageImportTagPlan(index)
	if err != nil {
		return nil, err
	}
	if len(choices) != len(plan) {
		return nil, errors.New("review every original archive tag, including skipped tags")
	}
	seen := map[string]bool{}
	byReference := map[string]imageImportTagReview{}
	for _, tag := range plan {
		byReference[tag.Reference] = tag
	}
	result := append([]imageImportChoice{}, choices...)
	for _, choice := range result {
		tag, exists := byReference[choice.Reference]
		if !exists || seen[choice.Reference] || choice.ExpectedID != "" && !imageIDValid(choice.ExpectedID) {
			return nil, errors.New("image import tag review identity differs")
		}
		seen[choice.Reference] = true
		matched := false
		for _, id := range tag.ConfigIDs {
			matched = matched || id == choice.ConfigID
		}
		if choice.ConfigID != "" && !matched || choice.Apply && (!matched || choice.ExpectedID != "" && !choice.Replace) || !choice.Apply && choice.Replace {
			return nil, errors.New("select the archived config and explicitly approve any existing-tag replacement")
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Reference < result[j].Reference })
	return result, nil
}
func imageImportCurrentTag(ctx context.Context, cli *client.Client, tag string) (string, error) {
	inspected, err := cli.ImageInspect(ctx, tag)
	if cerrdefs.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !imageIDValid(inspected.ID) {
		return "", errors.New("destination image has an unsupported identity")
	}
	return inspected.ID, nil
}
func imageImportOpen(source string) (*os.File, imageArchiveIndex, error) {
	var index imageArchiveIndex
	if !filepath.IsAbs(source) || len(source) < 3 || len(source) > 4096 || source[1] != ':' || strings.ContainsAny(source[2:], "\x00\r\n:") || !strings.EqualFold(filepath.Ext(source), ".tar") {
		return nil, index, errors.New("select a bounded plain .tar image archive on a local Windows drive")
	}
	before, err := os.Lstat(source)
	if err != nil {
		return nil, index, err
	}
	if !before.Mode().IsRegular() {
		return nil, index, errors.New("image import requires a regular archive file")
	}
	file, err := openImageArchiveFile(source)
	if err != nil {
		return nil, index, err
	}
	info, err := file.Stat()
	if err == nil && (!os.SameFile(before, info) || info.Size() != before.Size() || !info.ModTime().Equal(before.ModTime())) {
		err = errors.New("image archive changed while opening")
	}
	if err == nil {
		index, err = inspectImageArchive(file, info.Size())
	}
	after, afterErr := os.Lstat(source)
	if err == nil && (afterErr != nil || !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime())) {
		err = errors.New("image archive changed while reading")
	}
	if err != nil {
		return nil, index, errors.Join(err, file.Close())
	}
	return file, index, nil
}
func reviewImageImport(output io.Writer, daemon, source string) (resultErr error) {
	file, index, err := imageImportOpen(source)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	tags, err := imageImportTagPlan(index)
	if err != nil {
		return err
	}
	cli, ctx, cancel, err := transferClient(daemon)
	if err != nil {
		return err
	}
	defer cancel()
	defer func() { resultErr = errors.Join(resultErr, cli.Close()) }()
	for i := range tags {
		tags[i].CurrentID, err = imageImportCurrentTag(ctx, cli, tags[i].Reference)
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(output).Encode(imageImportReview{Version: 1, Source: source, SHA256: index.SHA256, Bytes: index.Bytes, Images: index.Images, Tags: tags})
}
