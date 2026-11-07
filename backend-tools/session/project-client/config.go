package main

// Compose resolves interpolation/includes/overrides. This adapter changes only
// documented filesystem fields in its resolved model, never application strings
// or environment values. The mapper confines local data to the selected project.
import (
	"errors"
	"fmt"
	"path"
	"strings"
)

type mappingReview struct {
	File     string `json:"file"`
	SHA256   string `json:"sha256"`
	Contents string `json:"contents"`
}
type workflowRequest struct {
	Source          string         `json:"source"`
	Shared          string         `json:"shared"`
	Signature       string         `json:"signature"`
	Daemon          string         `json:"daemon"`
	Docker          string         `json:"docker"`
	Config          string         `json:"config"`
	HelperDirectory string         `json:"helperDirectory"`
	Distribution    string         `json:"distribution"`
	Kind            string         `json:"kind"`
	File            string         `json:"file"`
	Overrides       []string       `json:"overrides"`
	EnvFiles        []string       `json:"envFiles"`
	Directory       string         `json:"directory"`
	Project         string         `json:"project"`
	Profiles        []string       `json:"profiles"`
	Services        []string       `json:"services"`
	Action          string         `json:"action"`
	Tag             string         `json:"tag"`
	Mapping         string         `json:"mapping"`
	ReviewedDaemon  string         `json:"reviewedDaemon"`
	MappingReview   *mappingReview `json:"mappingReview,omitempty"`
}

type pathKind string

const (
	pathFile      pathKind = "file"
	pathDirectory pathKind = "directory"
	pathBind      pathKind = "bind"
)

type pathMapper func(string, pathKind) (string, error)

func localContext(value string) bool {
	// Compose supports URL/service/image contexts; these are not filesystem paths.
	return !strings.Contains(value, "://") && !strings.HasPrefix(value, "service:") && !strings.HasPrefix(value, "target:") && !strings.HasPrefix(value, "git@")
}

func rewriteComposePaths(model map[string]any, mapPath pathMapper) error {
	rewrite := func(object map[string]any, key string, kind pathKind) error {
		value, exists := object[key]
		if !exists {
			return nil
		}
		text, ok := value.(string)
		if !ok || text == "" {
			return fmt.Errorf("unsupported Compose path field %s", key)
		}
		mapped, err := mapPath(text, kind)
		if err != nil {
			return err
		}
		object[key] = mapped
		return nil
	}
	services, ok := model["services"].(map[string]any)
	if !ok || len(services) > 1024 {
		return errors.New("Compose must resolve to at most 1,024 services")
	}
	for name, value := range services {
		service, ok := value.(map[string]any)
		if !ok {
			return errors.New("unsupported resolved Compose service")
		}
		if mounts, exists := service["volumes"]; exists {
			entries, ok := mounts.([]any)
			if !ok {
				return errors.New("unsupported resolved Compose mounts")
			}
			for _, value := range entries {
				attachment, ok := value.(map[string]any)
				if !ok {
					return errors.New("Compose mounts must resolve to long syntax")
				}
				if attachment["type"] == "bind" {
					// The mapper accepts either existing regular files or directories here.
					if err := rewrite(attachment, "source", pathBind); err != nil {
						return fmt.Errorf("service %s bind source: %w", name, err)
					}
					bind, ok := attachment["bind"].(map[string]any)
					if !ok {
						if value, exists := attachment["bind"]; exists && value != nil {
							return errors.New("unsupported Compose bind options")
						}
						bind = map[string]any{}
						attachment["bind"] = bind
					}
					bind["create_host_path"] = false
				}
			}
		}
		if value, exists := service["build"]; exists {
			build, ok := value.(map[string]any)
			if !ok {
				return errors.New("Compose build must resolve to object syntax")
			}
			context, ok := build["context"].(string)
			if !ok {
				return errors.New("Compose build context is missing")
			}
			local := localContext(context)
			if local {
				if err := rewrite(build, "context", pathDirectory); err != nil {
					return fmt.Errorf("service %s build context: %w", name, err)
				}
			}
			if local {
				if _, inline := build["dockerfile_inline"]; !inline {
					dockerfile := "Dockerfile"
					if value, exists := build["dockerfile"]; exists {
						var ok bool
						dockerfile, ok = value.(string)
						if !ok || dockerfile == "" {
							return errors.New("unsupported Compose Dockerfile")
						}
					}
					if !strings.HasPrefix(dockerfile, "/") {
						dockerfile = path.Join(build["context"].(string), dockerfile)
					}
					build["dockerfile"] = dockerfile
					if err := rewrite(build, "dockerfile", pathFile); err != nil {
						return err
					}
				}
			} else if dockerfile, ok := build["dockerfile"].(string); ok && strings.HasPrefix(dockerfile, "/") {
				return errors.New("remote context with a local absolute Dockerfile requires explicit separate build review")
			}
			if additional, exists := build["additional_contexts"]; exists {
				contexts, ok := additional.(map[string]any)
				if !ok {
					return errors.New("unsupported resolved additional build contexts")
				}
				for key, value := range contexts {
					text, ok := value.(string)
					if !ok {
						return errors.New("additional build context must be a string")
					}
					if localContext(text) {
						if err := rewrite(contexts, key, pathDirectory); err != nil {
							return err
						}
					}
				}
			}
		}
		// config normally resolves env files to environment values. If a future
		// upstream model retains one, refuse rather than send an unadapted guest path.
		if _, exists := service["env_file"]; exists {
			return errors.New("Compose retained unresolved env_file paths; this client model is not qualified")
		}
		if _, exists := service["develop"]; exists {
			return errors.New("Compose watch/develop paths require a separately qualified workflow")
		}
		if value, exists := service["devices"]; exists && value != nil {
			return errors.New("Compose device host paths require explicit engine-device review")
		}
		if service["use_api_socket"] == true {
			return errors.New("Compose use_api_socket would inject a user-distro socket; select the explicit owned shared socket workflow instead")
		}
	}
	for _, kind := range []string{"secrets", "configs"} {
		values, exists := model[kind]
		if !exists {
			continue
		}
		entries, ok := values.(map[string]any)
		if !ok {
			return errors.New("unsupported resolved Compose " + kind)
		}
		for _, value := range entries {
			entry, ok := value.(map[string]any)
			if !ok {
				return errors.New("unsupported Compose file-backed object")
			}
			if _, exists := entry["file"]; exists {
				if err := rewrite(entry, "file", pathFile); err != nil {
					return err
				}
			}
		}
	}
	// Local-driver bind options bypass service bind fields and cannot remain
	// silently pointed at another distro's /home. Named engine volumes still work.
	if volumes, ok := model["volumes"].(map[string]any); ok {
		for _, value := range volumes {
			if entry, ok := value.(map[string]any); ok {
				if options, exists := entry["driver_opts"]; exists && options != nil {
					return errors.New("Compose volume driver options require explicit engine-path review; use a service bind or ordinary named volume")
				}
			}
		}
	}
	return nil
}
