import { useRef, useState, type ReactNode } from "react";
import { isTauri } from "@tauri-apps/api/core";
import * as backend from "../../backend";
import { nodeTestcontainersEndpoint } from "../../model/client-targeting";
import { ClusterControls, EngineControls } from "../Operations";
import { engineState, canStartEngine } from "../shell/engine-state";
import { useElapsed, useEngineLifecycle } from "../shell/EngineLifecycle";
import type { ThemePreference } from "../shell/theme";
import { applyState, condition, kubernetesState } from "./app-settings-model";
import { StorageCleanup } from "./StorageCleanup";
import { currentSettings, type AppSettingsRead } from "./useAppSettings";

export const settingsTabs = [
  { id: "general", label: "General" },
  { id: "docker", label: "Docker engine" },
  { id: "kubernetes", label: "Kubernetes" },
  { id: "network", label: "Network" },
  { id: "storage", label: "Storage" },
  { id: "system", label: "WSL & system" },
  { id: "reset", label: "Reset" },
] as const;
export type SettingsTab = (typeof settingsTabs)[number]["id"];

function Row({ term, children }: { term: string; children: ReactNode }) {
  return (
    <>
      <dt>{term}</dt>
      <dd>{children}</dd>
    </>
  );
}

function ConfigurationState({
  read,
  onReload,
}: {
  read: AppSettingsRead;
  onReload: () => void;
}) {
  return (
    <p className="settings-source">
      {read.kind === "loading"
        ? "Reading backend configuration…"
        : read.kind === "ready"
          ? `Backend configuration read at ${new Date(read.readAt).toLocaleTimeString()}.`
          : read.kind === "error"
            ? `Backend configuration unavailable: ${read.message}`
            : "Backend configuration has not been read."}{" "}
      <button
        className="link-button"
        disabled={read.kind === "loading"}
        onClick={onReload}
      >
        Read again
      </button>
    </p>
  );
}

function EngineLifecycleControls({
  status,
  restart,
}: {
  status: backend.BackendStatus | null;
  restart?: boolean;
}) {
  const lifecycle = useEngineLifecycle();
  const elapsed = useElapsed(lifecycle.startedAt, lifecycle.action !== null);
  const connected = engineState(status) === "running";
  return (
    <>
      <div className="action-row">
        {lifecycle.action ? (
          <>
            <span className="inline-progress" role="status">
              {lifecycle.stage} {elapsed}s
            </span>
            <button className="button" onClick={lifecycle.cancel}>
              Cancel
            </button>
          </>
        ) : (
          <>
            {!connected && (
              <button
                className="button primary"
                disabled={!canStartEngine(status)}
                onClick={lifecycle.start}
              >
                Start engine
              </button>
            )}
            {connected && !restart && (
              <button className="button" onClick={lifecycle.requestStop}>
                Stop engine…
              </button>
            )}
            {restart && (
              <button
                className="button"
                disabled={!connected}
                onClick={lifecycle.requestRestart}
              >
                Restart engine…
              </button>
            )}
          </>
        )}
      </div>
      {!connected && !lifecycle.action && !canStartEngine(status) && (
        <p className="settings-hint">
          Start is unavailable until the backend is verified and its engine is
          offline. See Diagnostics.
        </p>
      )}
    </>
  );
}

export function Settings({
  tab,
  onTab,
  status,
  inventory,
  inventoryBusy,
  settingsRead,
  onReloadSettings,
  theme,
  onTheme,
  onMutation,
  onOpenWsl,
}: {
  tab: SettingsTab;
  onTab: (tab: SettingsTab) => void;
  status: backend.BackendStatus | null;
  inventory: backend.Inventory | null;
  inventoryBusy: boolean;
  settingsRead: AppSettingsRead;
  onReloadSettings: () => void;
  theme: ThemePreference;
  onTheme: (theme: ThemePreference) => void;
  onMutation: () => Promise<void>;
  onOpenWsl: () => void;
}) {
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const settings = currentSettings(settingsRead);
  const kubernetes = kubernetesState(settings);
  const connected = engineState(status) === "running";
  const [wsl, setWsl] = useState<backend.Diagnostic[] | null>(null);
  const [wslError, setWslError] = useState("");
  const [wslBusy, setWslBusy] = useState(false);
  const index = settingsTabs.findIndex((item) => item.id === tab);
  function focusTab(next: number) {
    const target =
      settingsTabs[(next + settingsTabs.length) % settingsTabs.length];
    onTab(target.id);
    tabRefs.current[settingsTabs.indexOf(target)]?.focus();
  }
  const kubernetesReady = settings
    ? condition(settings, "KubernetesReady")
    : undefined;
  const pending = settings
    ? applyState(settings)
    : { kind: "unknown" as const };
  return (
    <div className="settings">
      <div
        className="settings-tabs"
        role="tablist"
        aria-label="Settings sections"
        onKeyDown={(event) => {
          if (event.key === "ArrowRight" || event.key === "ArrowDown") {
            event.preventDefault();
            focusTab(index + 1);
          } else if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
            event.preventDefault();
            focusTab(index - 1);
          } else if (event.key === "Home") {
            event.preventDefault();
            focusTab(0);
          } else if (event.key === "End") {
            event.preventDefault();
            focusTab(settingsTabs.length - 1);
          }
        }}
      >
        {settingsTabs.map((item, position) => (
          <button
            key={item.id}
            ref={(element) => {
              tabRefs.current[position] = element;
            }}
            role="tab"
            id={`settings-tab-${item.id}`}
            aria-selected={item.id === tab}
            aria-controls="settings-panel"
            tabIndex={item.id === tab ? 0 : -1}
            onClick={() => onTab(item.id)}
          >
            {item.label}
          </button>
        ))}
      </div>
      <div
        className="settings-panel"
        id="settings-panel"
        role="tabpanel"
        aria-labelledby={`settings-tab-${tab}`}
        tabIndex={0}
      >
        {tab === "general" && (
          <>
            <h2>Appearance</h2>
            <div className="segmented" role="radiogroup" aria-label="Theme">
              {(["system", "light", "dark"] as const).map((value) => (
                <label key={value}>
                  <input
                    type="radio"
                    name="theme"
                    value={value}
                    checked={theme === value}
                    onChange={() => onTheme(value)}
                  />
                  <span>
                    {value === "system"
                      ? "Match Windows"
                      : value === "light"
                        ? "Light"
                        : "Dark"}
                  </span>
                </label>
              ))}
            </div>
            <p className="settings-hint">
              Applies to OrbWin content. The Windows title bar follows the
              Windows app theme.
            </p>
            <h2>Engine</h2>
            <dl className="properties">
              <Row term="Status">
                {connected
                  ? `Running${status?.version ? ` · Docker ${status.version}` : ""}`
                  : (status?.message ?? "Not checked yet")}
              </Row>
              <Row term="Desired state">
                {settings?.running === undefined
                  ? "Unknown (backend configuration unavailable)"
                  : settings.running
                    ? "Running"
                    : "Stopped"}
              </Row>
              <Row term="Closing the window">
                Closing OrbWin leaves the engine in its current state. Stop it
                here or from the sidebar switch. There is no tray icon yet;
                reopen OrbWin to manage the engine.
              </Row>
              <Row term="Start with Windows">
                Not available yet. The engine starts only when you start it.
              </Row>
            </dl>
            <EngineLifecycleControls status={status} />
            <ConfigurationState
              read={settingsRead}
              onReload={onReloadSettings}
            />
          </>
        )}
        {tab === "docker" && (
          <>
            <h2>Docker engine</h2>
            <dl className="properties">
              <Row term="Instance">orbwin</Row>
              <Row term="Engine">
                {settings?.containerEngine ?? "Unknown"}
                {status?.version ? ` · Docker ${status.version}` : ""}
              </Row>
              <Row term="Docker endpoint">
                <code>{backend.endpoint}</code>
              </Row>
              <Row term="Client targeting">
                OrbWin does not create or switch a Docker context and does not
                change DOCKER_HOST globally. Target it per command or per
                session:
                <pre className="settings-code">
                  {`docker --host ${backend.endpoint} ps\n$env:DOCKER_HOST = "${backend.endpoint}"`}
                </pre>
              </Row>
              <Row term="Testcontainers Node 12.2.1">
                <code>{nodeTestcontainersEndpoint}</code>
                <p>
                  Use this DOCKER_HOST value for the Windows Node client,
                  TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 and
                  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock.
                  Keep Ryuk enabled. Other client versions require
                  qualification.
                </p>
              </Row>
              <Row term="Coexistence">
                Docker Desktop and Rancher Desktop keep their own endpoints.
                Simultaneous-engine operation is not yet qualified (BL-001).
              </Row>
            </dl>
            <h2>Restart</h2>
            <p>
              Restarting interrupts running containers. It runs a stop followed
              by a start through the backend.
            </p>
            <EngineLifecycleControls status={status} restart />
            <h2>Raw configuration</h2>
            <p>Read the backend App resource as JSON without changing it.</p>
            <EngineControls lifecycle={false} />
          </>
        )}
        {tab === "kubernetes" && (
          <>
            <h2>Kubernetes</h2>
            <p>
              The optional single-node cluster runs inside the OrbWin guest.
              Docker workflows work without it.
            </p>
            <dl className="properties">
              <Row term="State">
                {kubernetes === "disabled"
                  ? "Disabled"
                  : kubernetes === "ready"
                    ? "Enabled · ready"
                    : kubernetes === "starting"
                      ? `Enabled · not ready${kubernetesReady?.reason ? ` (${kubernetesReady.reason})` : ""}`
                      : kubernetes === "enabled"
                        ? "Enabled · readiness not reported yet"
                        : "Unknown (backend configuration unavailable)"}
              </Row>
              {kubernetesReady?.message && kubernetes !== "ready" && (
                <Row term="Backend message">{kubernetesReady.message}</Row>
              )}
              <Row term="Version">
                {settings?.kubernetesVersion ?? "Backend default"}
                {" · version selection is not available yet"}
              </Row>
              <Row term="kubeconfig">
                OrbWin uses its own mirrored kubeconfig from the owned backend
                home for every cluster view and command. It does not switch your
                current kubectl context. In this development build the backend
                runs with the owned home as its profile, so the
                rancher-desktop-orbwin context is merged there, not into your
                own ~/.kube/config; this is not yet qualified for an installed
                build.
              </Row>
              <Row term="Service access">
                Port forwards opened from Pods and Services listen on localhost
                only.
              </Row>
            </dl>
            {pending.kind === "applying" && (
              <p className="notice" role="status">
                The backend is still applying the last change
                {pending.reason ? ` (${pending.reason})` : ""}. Enabling can
                download components and take several minutes.
              </p>
            )}
            <ClusterControls
              only={
                kubernetes === "unknown"
                  ? undefined
                  : kubernetes === "disabled"
                    ? "enable"
                    : "disable"
              }
            />
            <ConfigurationState
              read={settingsRead}
              onReload={onReloadSettings}
            />
          </>
        )}
        {tab === "network" && (
          <>
            <h2>Port publishing</h2>
            <dl className="properties">
              <Row term="Default bind address">
                Ports published from the Run form bind 127.0.0.1 unless you
                enter another IPv4 address. A container port alone gets a
                dynamic localhost port.
              </Row>
              <Row term="Port conflicts">
                Behavior when another engine or Windows app already listens on
                the host port is not yet qualified, so a conflict may not be
                reported clearly. If a published port does not answer, check for
                another listener (for example{" "}
                <code>netstat -ano | findstr :8080</code>) and choose another
                port.
              </Row>
              <Row term="Container IP access">
                Direct access to container IPs from Windows is not provided. Use
                published ports.
              </Row>
            </dl>
            <h2>Domains and HTTPS</h2>
            <p className="notice">
              Automatic local domain names for containers and services, and
              trusted local HTTPS, are planned for phase 2. They need a Windows
              resolver integration and a local certificate authority, which
              change system DNS and certificate trust. These must be prototyped
              for VPN, managed-device and conflict behavior before OrbWin
              changes them for you (project decisions P09, P10).
            </p>
          </>
        )}
        {tab === "storage" && (
          <StorageCleanup
            inventory={inventory}
            disabled={!inventory || inventoryBusy}
            onMutation={onMutation}
          />
        )}
        {tab === "system" && (
          <>
            <h2>Resources</h2>
            <dl className="properties">
              <Row term="Requested CPUs">{settings?.cpus ?? "Unknown"}</Row>
              <Row term="Requested memory">{settings?.memory ?? "Unknown"}</Row>
            </dl>
            <p className="notice">
              CPU and memory editing is unavailable until WSL resource scope is
              qualified (BL-006). The requested values above were not observed
              to limit the WSL environment. No global WSL settings are changed.
            </p>
            <h2>WSL</h2>
            <p>
              OrbWin runs its engine in its own WSL distribution. Your
              development distributions remain independent.
            </p>
            <div className="action-row">
              <button
                className="button"
                disabled={wslBusy}
                onClick={() => {
                  if (!isTauri()) {
                    setWslError(
                      "WSL is read by the Windows desktop application.",
                    );
                    return;
                  }
                  setWslBusy(true);
                  setWslError("");
                  void backend
                    .diagnostics()
                    .then(setWsl)
                    .catch((error) => setWslError(String(error)))
                    .finally(() => setWslBusy(false));
                }}
              >
                {wslBusy ? "Reading WSL…" : "Read WSL version"}
              </button>
              <button className="button" onClick={onOpenWsl}>
                Open WSL distributions
              </button>
            </div>
            {wslError && <p role="status">{wslError}</p>}
            {wsl?.map((item) => (
              <section key={item.name} className="diagnostic-result">
                <h3>
                  {item.name} · {item.success ? "Read successfully" : "Failed"}
                </h3>
                <pre>{item.output}</pre>
              </section>
            ))}
          </>
        )}
        {tab === "reset" && (
          <>
            <h2>Reset</h2>
            <p>
              Two scopes are distinguished. Neither is available in this build.
            </p>
            <section className="reset-option">
              <div>
                <h3>Delete Docker data</h3>
                <p>
                  Would remove every container, image, volume, network and build
                  cache in the OrbWin engine, keeping Kubernetes and settings.
                  Not available: the backend adapter has no qualified
                  engine-data reset. To free space now, use Storage › Clean up
                  unused resources.
                </p>
              </div>
              <button
                className="button danger"
                disabled
                aria-describedby="reset-reason"
              >
                Delete Docker data…
              </button>
            </section>
            <section className="reset-option">
              <div>
                <h3>Reset OrbWin</h3>
                <p>
                  Would delete all Docker data, the Kubernetes cluster, the
                  OrbWin WSL distribution and its disks, and OrbWin settings.
                  Not available: deleting the owned WSL registration has not
                  been qualified to leave other engines and your distributions
                  untouched (BL-002).
                </p>
              </div>
              <button
                className="button danger"
                disabled
                aria-describedby="reset-reason"
              >
                Reset OrbWin…
              </button>
            </section>
            <p className="settings-hint" id="reset-reason">
              Reset actions are disabled until scoped, data-preserving reset is
              implemented and tested. Never use a global WSL shutdown or
              unregister of other distributions to reset OrbWin.
            </p>
          </>
        )}
      </div>
    </div>
  );
}
