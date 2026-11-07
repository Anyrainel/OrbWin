package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const migrationInventoryItems = 4096

type migrationImage struct {
	ID    string   `json:"id"`
	Tags  []string `json:"tags"`
	Bytes int64    `json:"bytes"`
}
type migrationVolume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Scope      string `json:"scope"`
	PlainLocal bool   `json:"plainLocal"`
}
type migrationProjectMember struct {
	Container        string                  `json:"container"`
	Image            string                  `json:"image"`
	Project          string                  `json:"project"`
	Service          string                  `json:"service"`
	WorkingDirectory string                  `json:"workingDirectory"`
	ConfigFilesLabel string                  `json:"configFilesLabel"`
	ExplicitEnvFiles bool                    `json:"explicitEnvFiles"`
	BindMounts       bool                    `json:"bindMounts"`
	OneOff           bool                    `json:"oneOff"`
	Volumes          []string                `json:"volumes"`
	Mounts           []migrationProjectMount `json:"mounts"`
}
type migrationProjectMount struct {
	Kind             string `json:"kind"`
	Volume           string `json:"volume,omitempty"`
	Target           string `json:"target"`
	ReadOnly         bool   `json:"readOnly"`
	MappingSupported bool   `json:"mappingSupported"`
}

func projectMigrationMount(mount container.MountPoint, roots map[string]string) migrationProjectMount {
	kind := string(mount.Type)
	if kind != "volume" && kind != "bind" && kind != "tmpfs" {
		kind = "other"
	}
	projected := migrationProjectMount{Kind: kind, Target: mount.Destination, ReadOnly: !mount.RW}
	if kind == "volume" {
		projected.Volume = mount.Name
		projected.MappingSupported = mount.Source != "" && mount.Source == roots[mount.Name]
	}
	return projected
}

type migrationInventory struct {
	Version        int                      `json:"version"`
	Source         migrationSourceIdentity  `json:"source"`
	Target         string                   `json:"target"`
	Images         []migrationImage         `json:"images"`
	Volumes        []migrationVolume        `json:"volumes"`
	ProjectMembers []migrationProjectMember `json:"projectMembers"`
	Warnings       []string                 `json:"warnings"`
}

func migrationText(value string, limit int) bool {
	return len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}

// Inventory is a bounded observation, not a transferable snapshot. Label paths
// are returned verbatim for later review: do not split, open, execute, or assume
// the order/availability of files from labels. No env, credentials, mount host
// paths, volume options, arbitrary labels or cluster/settings state are emitted.
func inspectDesktopMigration(ctx context.Context, cli *client.Client, target string) (migrationInventory, error) {
	return inspectMigrationSource(ctx, cli, target, func(expected string) (migrationSourceIdentity, error) {
		return verifyDesktopMigrationSource(ctx, cli, target, expected)
	})
}
func inspectMigrationSource(ctx context.Context, cli *client.Client, target string, verify func(string) (migrationSourceIdentity, error)) (migrationInventory, error) {
	result := migrationInventory{Version: 1, Target: target, Images: []migrationImage{}, Volumes: []migrationVolume{}, ProjectMembers: []migrationProjectMember{}, Warnings: []string{"Inventory is not atomic. Recheck each selected resource before transfer; running applications require their own consistency procedure.", "Compose label paths require review on this computer. Project files, images and volumes are separate selections; credentials, settings and Kubernetes state are not transferred."}}
	identity, err := verify("")
	if err != nil {
		return result, err
	}
	result.Source = identity
	images, err := cli.ImageList(ctx, client.ImageListOptions{All: true})
	if err != nil {
		return result, err
	}
	if len(images.Items) > migrationInventoryItems {
		return result, errors.New("source has more than 4096 images; inventory refused without truncation")
	}
	for _, image := range images.Items {
		if !imageIDValid(image.ID) || len(image.RepoTags) > 512 {
			return result, errors.New("source image identity/tag inventory exceeds supported bounds")
		}
		tags := append([]string{}, image.RepoTags...)
		for _, tag := range tags {
			if !migrationText(tag, 512) {
				return result, errors.New("source image tag exceeds supported bounds")
			}
		}
		sort.Strings(tags)
		result.Images = append(result.Images, migrationImage{ID: image.ID, Tags: tags, Bytes: image.Size})
	}
	volumes, err := cli.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return result, err
	}
	if len(volumes.Warnings) != 0 {
		return result, errors.New("source volume inventory returned warnings; review source engine before migration")
	}
	if len(volumes.Items) > migrationInventoryItems {
		return result, errors.New("source has more than 4096 volumes; inventory refused without truncation")
	}
	volumeRoots := map[string]string{}
	for _, volume := range volumes.Items {
		if !migrationText(volume.Name, 255) || volume.Name == "" || !migrationText(volume.Driver, 128) || !migrationText(volume.Scope, 128) {
			return result, errors.New("source volume inventory contains unsupported metadata")
		}
		result.Volumes = append(result.Volumes, migrationVolume{Name: volume.Name, Driver: volume.Driver, Scope: volume.Scope, PlainLocal: volume.Driver == "local" && volume.Scope == "local" && len(volume.Options) == 0})
		if volume.Driver == "local" && volume.Scope == "local" && len(volume.Options) == 0 && volume.Mountpoint != "" {
			volumeRoots[volume.Name] = volume.Mountpoint
		}
	}
	containers, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return result, err
	}
	if len(containers.Items) > migrationInventoryItems {
		return result, errors.New("source has more than 4096 containers; project inventory refused without truncation")
	}
	mountCount := 0
	for _, item := range containers.Items {
		project := item.Labels["com.docker.compose.project"]
		if project == "" {
			continue
		}
		member := migrationProjectMember{Container: item.ID, Project: project, Service: item.Labels["com.docker.compose.service"], WorkingDirectory: item.Labels["com.docker.compose.project.working_dir"], ConfigFilesLabel: item.Labels["com.docker.compose.project.config_files"], ExplicitEnvFiles: item.Labels["com.docker.compose.project.environment_file"] != "", OneOff: strings.EqualFold(item.Labels["com.docker.compose.oneoff"], "true"), Volumes: []string{}, Mounts: []migrationProjectMount{}}
		if item.ImageID != "" && !imageIDValid(item.ImageID) {
			return result, errors.New("source member image identity is invalid")
		}
		member.Image = item.ImageID
		if !fullContainerID(item.ID) || !migrationText(project, 255) || !migrationText(member.Service, 255) || !migrationText(member.WorkingDirectory, 4096) || !migrationText(member.ConfigFilesLabel, 16384) || len(item.Mounts) > 512 {
			return result, errors.New("source Compose metadata exceeds supported bounds")
		}
		for _, mount := range item.Mounts {
			mountCount++
			if mountCount > 8192 {
				return result, errors.New("source project mounts exceed 8192 observations; no partial mapping was returned")
			}
			if !migrationText(mount.Destination, 4096) || mount.Destination == "" {
				return result, errors.New("source mount target is missing or exceeds supported bounds")
			}
			projected := projectMigrationMount(mount, volumeRoots)
			member.Mounts = append(member.Mounts, projected)
			if mount.Type == "bind" {
				member.BindMounts = true
			}
			if mount.Type != "volume" {
				continue
			}
			if !migrationText(mount.Name, 255) || mount.Name == "" {
				return result, errors.New("source project volume name exceeds supported bounds")
			}
			member.Volumes = append(member.Volumes, mount.Name)
		}
		sort.Strings(member.Volumes)
		result.ProjectMembers = append(result.ProjectMembers, member)
	}
	if _, err := verify(identity.Daemon); err != nil {
		return result, err
	}
	sort.Slice(result.Images, func(i, j int) bool { return result.Images[i].ID < result.Images[j].ID })
	sort.Slice(result.Volumes, func(i, j int) bool { return result.Volumes[i].Name < result.Volumes[j].Name })
	sort.Slice(result.ProjectMembers, func(i, j int) bool { return result.ProjectMembers[i].Container < result.ProjectMembers[j].Container })
	return result, nil
}

func reviewDesktopMigration(output io.Writer, target string) error {
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
	ctx, cancelSource := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelSource()
	result, err := inspectDesktopMigration(ctx, cli, target)
	if err != nil {
		return err
	}
	// Emit only a complete, identity-rechecked inventory, never a partial preview.
	info, err := owned.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != target || info.Info.OSType != "linux" {
		return errors.New("owned target changed during migration review; inventory refused")
	}
	return json.NewEncoder(output).Encode(result)
}
