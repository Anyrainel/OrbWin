package localnet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func namespaceFixture() NamespaceIntent {
	return NamespaceIntent{Session: strings.Repeat("a", 32), ImageID: "sha256:" + strings.Repeat("b", 64), AssetSHA: strings.Repeat("c", 64), ContainerID: strings.Repeat("d", 64), StartedAt: "2026-10-24T12:00:00Z"}
}
func TestNamespaceConfigurationHasNoTargetFilesystemPIDOrPrivilege(t *testing.T) {
	intent := namespaceFixture()
	options, err := NamespaceConfiguration(intent)
	if err != nil {
		t.Fatal(err)
	}
	configuration, host := options.Config, options.HostConfig
	if configuration.User != "65532:65532" || configuration.Image != intent.ImageID || len(configuration.Entrypoint) != 0 || configuration.OpenStdin || configuration.Tty || len(configuration.Volumes) != 0 || strings.Join(configuration.Cmd, " ") != "/orbwin-services netns "+intent.Session {
		t.Fatal("unexpected namespace executable/profile")
	}
	if host.NetworkMode != container.NetworkMode("container:"+intent.ContainerID) || host.PidMode != "" || host.Privileged || !host.ReadonlyRootfs || !host.AutoRemove || len(host.CapAdd) != 0 || len(host.CapDrop) != 1 || host.CapDrop[0] != "ALL" || len(host.Mounts) != 0 || len(host.Binds) != 0 || len(host.PortBindings) != 0 || len(host.SecurityOpt) != 1 {
		t.Fatal("namespace helper gained target storage/PID or extra privileges")
	}
	for _, invalid := range []NamespaceIntent{{}, {Session: intent.Session, ImageID: "mutable:latest", AssetSHA: intent.AssetSHA, ContainerID: intent.ContainerID, StartedAt: intent.StartedAt}} {
		if _, err := NamespaceConfiguration(invalid); err == nil {
			t.Fatal("invalid ownership intent generated create options")
		}
	}
}

func TestNamespaceVerificationRefusesChangedIdentityAndIsolation(t *testing.T) {
	for _, fault := range []string{"", "foreign-label", "wrong-image", "wrong-name", "paused-helper", "stopped-helper", "restart-target", "paused-target", "stopped-target", "host-target", "shared-target", "host-pid", "capability", "bind", "entrypoint", "tmpfs", "memory", "security-option", "host-ipc"} {
		t.Run(fault, func(t *testing.T) {
			intent := namespaceFixture()
			helperID := strings.Repeat("e", 64)
			options, _ := NamespaceConfiguration(intent)
			state := map[string]any{"Running": true, "StartedAt": intent.StartedAt}
			targetState := map[string]any{"Running": true, "StartedAt": intent.StartedAt}
			targetNetwork := "bridge"
			imageID, name := intent.ImageID, "/"+intent.name()
			switch fault {
			case "foreign-label":
				options.Config.Labels["dev.orbwin.session"] = strings.Repeat("f", 32)
			case "wrong-image":
				imageID = "sha256:" + strings.Repeat("f", 64)
			case "wrong-name":
				name = "/other"
			case "paused-helper":
				state["Paused"] = true
			case "stopped-helper":
				state["Running"] = false
			case "restart-target":
				targetState["StartedAt"] = "2026-10-24T12:01:00Z"
			case "paused-target":
				targetState["Paused"] = true
			case "stopped-target":
				targetState["Running"] = false
			case "host-target":
				targetNetwork = "host"
			case "shared-target":
				targetNetwork = "container:" + strings.Repeat("f", 64)
			case "host-pid":
				options.HostConfig.PidMode = "host"
			case "capability":
				options.HostConfig.CapAdd = []string{"SYS_ADMIN"}
			case "bind":
				options.HostConfig.Binds = []string{"/:/host"}
			case "entrypoint":
				options.Config.Entrypoint = []string{"/foreign"}
			case "tmpfs":
				options.HostConfig.Tmpfs["/other"] = "rw"
			case "memory":
				options.HostConfig.Memory = 0
			case "security-option":
				options.HostConfig.SecurityOpt = nil
			case "host-ipc":
				options.HostConfig.IpcMode = "host"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method != "GET" {
					t.Error("verification attempted a mutation")
					http.Error(w, "unexpected mutation", 500)
					return
				}
				switch request.URL.Path {
				case "/v1.56/containers/" + intent.ContainerID + "/json":
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": intent.ContainerID, "State": targetState, "Config": map[string]any{}, "HostConfig": map[string]string{"NetworkMode": targetNetwork}})
				case "/v1.56/containers/" + helperID + "/json":
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": helperID, "Name": name, "Image": imageID, "Config": options.Config, "HostConfig": options.HostConfig, "State": state})
				default:
					t.Errorf("unselected inspect: %s", request.URL.Path)
					http.NotFound(w, request)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.56"))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			if fault == "" {
				if err := VerifyServiceTarget(context.Background(), cli, intent); err != nil {
					t.Fatal(err)
				}
				if err := VerifyNamespace(context.Background(), cli, intent, helperID); err != nil {
					t.Fatal(err)
				}
			} else {
				// Dial must stop at a failed read-only check, before ExecCreate.
				if connection, err := DialVerifiedNamespace(context.Background(), cli, intent, helperID, 8080); err == nil || connection != nil {
					t.Fatal("changed target/helper reached a dial")
				}
				if fault == "stopped-helper" || fault == "paused-helper" || fault == "stopped-target" || fault == "restart-target" {
					if err := VerifyNamespaceOwnership(context.Background(), cli, intent, helperID); err != nil {
						t.Fatalf("inactive owned helper could not be reviewed for cleanup: %v", err)
					}
				}
			}
		})
	}
}
