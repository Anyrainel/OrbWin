import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { RefreshCw, Search, X } from "lucide-react";
import * as backend from "./backend";
import {
  navigation,
  isResourcePage,
  shortcutFor,
  shortcutPages,
  type Page,
} from "./model/navigation";
import { ResourceList, Unavailable } from "./components/ResourceInventory";
import { Cluster } from "./components/Cluster";
import { Wsl } from "./components/Wsl";
import { isKubernetesContainer } from "./model/resources";
import {
  BuildControls,
  OperationProvider,
  ProjectControls,
  ResourceCreate,
} from "./components/Operations";
import { ToastProvider, useToast } from "./components/shell/Toasts";
import {
  EngineLifecycleProvider,
  useEngineLifecycle,
} from "./components/shell/EngineLifecycle";
import { EngineSetup } from "./components/shell/EngineSetup";
import { Sidebar } from "./components/shell/Sidebar";
import { ApplyBanner } from "./components/shell/ApplyBanner";
import {
  CommandPalette,
  type PaletteCommand,
} from "./components/shell/CommandPalette";
import { shortcutCommand } from "./components/shell/shortcuts";
import { useTheme } from "./components/shell/theme";
import { canStartEngine, engineState } from "./components/shell/engine-state";
import {
  Settings,
  settingsTabs,
  type SettingsTab,
} from "./components/settings/Settings";
import {
  currentSettings,
  useAppSettings,
} from "./components/settings/useAppSettings";
import { kubernetesState } from "./components/settings/app-settings-model";
import "./styles/shell.css";

export function App() {
  return (
    <ToastProvider>
      <Engine />
    </ToastProvider>
  );
}

// Owns the status/inventory reads. Mutations from operation dialogs, cleanup
// and engine lifecycle all reconcile through the same refresh.
function Engine() {
  const notify = useToast();
  const [status, setStatus] = useState<backend.BackendStatus | null>(null);
  const [inventory, setInventory] = useState<backend.Inventory | null>(null);
  const [busy, setBusy] = useState(false);
  const active = useRef(false);
  const idle = useRef<(() => void)[]>([]);
  const generation = useRef(0);
  const settings = useAppSettings();
  // Cancelling invalidates this read's result. Native bounded subprocesses finish
  // independently; no engine lifecycle or workload operation is cancelled.
  function cancel() {
    generation.current++;
    notify(
      "info",
      "Refresh cancelled. Waiting for the bounded read to finish. No engine state was changed.",
    );
  }
  async function refresh() {
    if (active.current) return;
    active.current = true;
    const id = ++generation.current;
    setBusy(true);
    try {
      const next = await backend.status();
      if (id !== generation.current) return;
      setStatus(next);
      if (next.available) {
        const data = await backend.inventory();
        if (id === generation.current) setInventory(data);
      } else setInventory(null);
    } catch (reason) {
      if (id === generation.current) {
        notify("error", `Could not read the engine: ${String(reason)}`);
        setInventory(null);
        setStatus(null);
      }
    } finally {
      active.current = false;
      setBusy(false);
      for (const settled of idle.current.splice(0)) settled();
    }
  }
  async function reconcileMutation() {
    generation.current++;
    if (active.current)
      await new Promise<void>((resolve) => idle.current.push(resolve));
    await refresh();
  }
  useEffect(() => {
    void refresh();
  }, []);
  const connected = status?.available === true;
  // Desired/observed backend configuration follows connection changes.
  useEffect(() => {
    void settings.reload();
  }, [connected, settings.reload]);
  const lifecycleSettled = useCallback(async () => {
    await reconcileMutation();
    void settings.reload();
  }, [settings.reload]);
  return (
    <OperationProvider onMutation={reconcileMutation}>
      <EngineLifecycleProvider onSettled={lifecycleSettled}>
        <Shell
          status={status}
          inventory={inventory}
          busy={busy}
          refresh={() => void refresh()}
          cancel={cancel}
          reconcile={reconcileMutation}
          settings={settings}
        />
      </EngineLifecycleProvider>
    </OperationProvider>
  );
}

function Shell({
  status,
  inventory,
  busy,
  refresh,
  cancel,
  reconcile,
  settings,
}: {
  status: backend.BackendStatus | null;
  inventory: backend.Inventory | null;
  busy: boolean;
  refresh: () => void;
  cancel: () => void;
  reconcile: () => Promise<void>;
  settings: ReturnType<typeof useAppSettings>;
}) {
  const notify = useToast();
  const lifecycle = useEngineLifecycle();
  const [page, setPage] = useState<Page>("containers");
  const [settingsTab, setSettingsTab] = useState<SettingsTab>("general");
  const [palette, setPalette] = useState(false);
  const [theme, setTheme] = useTheme();
  const [query, setQuery] = useState("");
  const [includeKubernetes, setIncludeKubernetes] = useState(false);
  const [diagnostics, setDiagnostics] = useState<backend.Diagnostic[]>([]);
  const [diagnosticBusy, setDiagnosticBusy] = useState(false);
  const search = useRef<HTMLInputElement>(null);
  function navigate(next: Page) {
    setPage(next);
    setQuery("");
  }
  function openSettings(tab: SettingsTab) {
    setSettingsTab(tab);
    navigate("settings");
  }
  useEffect(() => {
    const key = (event: KeyboardEvent) => {
      const target = event.target as Element | null;
      const command = shortcutCommand(event, target);
      const modal = document.querySelector("dialog[open]");
      if (command?.kind === "palette") {
        // Ctrl+K toggles the palette but never opens it over another dialog.
        if (palette) {
          event.preventDefault();
          setPalette(false);
        } else if (!modal) {
          event.preventDefault();
          setPalette(true);
        }
        return;
      }
      if (modal) return;
      if (command?.kind === "settings") {
        event.preventDefault();
        navigate("settings");
        return;
      }
      if (command?.kind === "page") {
        const next = shortcutPages[command.index];
        if (next) {
          event.preventDefault();
          navigate(next);
        }
        return;
      }
      if (
        event.ctrlKey &&
        event.key.toLowerCase() === "f" &&
        isResourcePage(page)
      ) {
        event.preventDefault();
        search.current?.focus();
      }
      if (event.key === "Escape") {
        if (query) setQuery("");
        else if (busy) cancel();
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [page, query, busy, palette]);
  async function inspect() {
    if (diagnosticBusy) return;
    setDiagnosticBusy(true);
    try {
      setDiagnostics(await backend.diagnostics());
    } catch (reason) {
      notify("error", String(reason));
    } finally {
      setDiagnosticBusy(false);
    }
  }
  function saveSnapshot() {
    setDiagnosticBusy(true);
    void backend
      .exportSupport()
      .then((saved) => {
        if (saved)
          notify("success", "Diagnostic snapshot saved to the selected file.");
      })
      .catch((reason) => notify("error", String(reason)))
      .finally(() => setDiagnosticBusy(false));
  }
  const current = navigation.find((n) => n.id === page)!;
  const connected = engineState(status) === "running";
  const appSettings = currentSettings(settings.read);
  const kubernetes = kubernetesState(appSettings);
  const resourceRows =
    inventory && isResourcePage(page)
      ? page === "containers" && !includeKubernetes
        ? inventory.containers.filter((row) => !isKubernetesContainer(row))
        : inventory[page]
      : [];
  const kubernetesContainers =
    inventory?.containers.filter(isKubernetesContainer).length ?? 0;
  const runningContainers = inventory
    ? inventory.containers.filter(
        (row) =>
          !isKubernetesContainer(row) &&
          String(row.State ?? "").toLowerCase() === "running",
      ).length
    : null;
  const lifecycleBusy = lifecycle.action
    ? "Another engine request is in progress."
    : undefined;
  const commands = useMemo<PaletteCommand[]>(
    () => [
      ...navigation.map(({ id, label }) => ({
        id: `page-${id}`,
        section: "Go to",
        label,
        shortcut: shortcutFor(id),
        run: () => navigate(id),
      })),
      ...settingsTabs.map(({ id, label }) => ({
        id: `settings-${id}`,
        section: "Settings",
        label,
        run: () => openSettings(id),
      })),
      {
        id: "refresh",
        section: "Engine",
        label: "Refresh engine status",
        disabledReason: busy ? "A refresh is already running." : undefined,
        run: refresh,
      },
      {
        id: "start",
        section: "Engine",
        label: "Start engine",
        disabledReason:
          lifecycleBusy ??
          (connected
            ? "The engine is already running."
            : canStartEngine(status)
              ? undefined
              : "The backend must be verified and its engine offline."),
        run: lifecycle.start,
      },
      {
        id: "stop",
        section: "Engine",
        label: "Stop engine…",
        disabledReason:
          lifecycleBusy ??
          (connected ? undefined : "The engine is not running."),
        run: lifecycle.requestStop,
      },
      {
        id: "restart",
        section: "Engine",
        label: "Restart engine…",
        disabledReason:
          lifecycleBusy ??
          (connected ? undefined : "The engine is not running."),
        run: lifecycle.requestRestart,
      },
      {
        id: "kubernetes",
        section: "Kubernetes",
        label:
          kubernetes === "disabled"
            ? "Enable Kubernetes…"
            : "Kubernetes settings",
        run: () => openSettings("kubernetes"),
      },
      {
        id: "cleanup",
        section: "Storage",
        label: "Clean up unused resources…",
        run: () => openSettings("storage"),
      },
      {
        id: "snapshot",
        section: "Diagnostics",
        label: "Save diagnostic snapshot…",
        disabledReason: diagnosticBusy
          ? "A snapshot is in progress."
          : undefined,
        run: saveSnapshot,
      },
      ...(["system", "light", "dark"] as const).map((value) => ({
        id: `theme-${value}`,
        section: "Appearance",
        label:
          value === "system"
            ? "Match Windows theme"
            : value === "light"
              ? "Light theme"
              : "Dark theme",
        disabledReason: theme === value ? "Already selected." : undefined,
        run: () => setTheme(value),
      })),
    ],
    [
      busy,
      connected,
      status,
      lifecycle,
      lifecycleBusy,
      kubernetes,
      diagnosticBusy,
      theme,
    ],
  );
  return (
    <div className="app-shell">
      <a href="#main-content" className="skip-link">
        Skip to content
      </a>
      <Sidebar
        page={page}
        onNavigate={navigate}
        runningContainers={runningContainers}
        kubernetes={kubernetes}
        onEnableKubernetes={() => openSettings("kubernetes")}
        onPalette={() => setPalette(true)}
        status={status}
        checking={busy}
      />
      <main id="main-content" tabIndex={-1}>
        <ApplyBanner
          read={settings.read}
          onReload={() => void settings.reload()}
        />
        {isResourcePage(page) && status && !connected ? (
          <EngineSetup
            status={status}
            checking={busy}
            onRefresh={refresh}
            onDiagnostics={() => navigate("diagnostics")}
          />
        ) : isResourcePage(page) ? (
          <section className="resource-workspace" aria-label={current.label}>
            <div className="list-pane">
              <header className="list-header">
                <div>
                  <h1>{current.label}</h1>
                  <small>
                    {busy
                      ? "Reading engine…"
                      : inventory
                        ? `${resourceRows.length} ${page === "containers" ? "containers" : "items"}`
                        : "Unavailable"}
                  </small>
                </div>
                <ResourceCreate page={page} disabled={!inventory || busy} />
                <button
                  className="icon-button"
                  aria-label={busy ? "Cancel refresh" : "Refresh inventory"}
                  title={busy ? "Cancel refresh" : "Refresh inventory"}
                  onClick={() => (busy ? cancel() : refresh())}
                >
                  {busy ? <X size={17} /> : <RefreshCw size={17} />}
                </button>
              </header>
              <label className="search">
                <Search size={15} />
                <input
                  ref={search}
                  aria-label={`Filter ${current.label.toLowerCase()}`}
                  placeholder="Search"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  disabled={!inventory}
                />
                {query && (
                  <button
                    aria-label="Clear filter"
                    onClick={() => {
                      setQuery("");
                      search.current?.focus();
                    }}
                  >
                    <X size={13} />
                  </button>
                )}
                <kbd>Ctrl F</kbd>
              </label>
              {page === "containers" && kubernetesContainers > 0 && (
                <label className="inventory-option">
                  <input
                    type="checkbox"
                    checked={includeKubernetes}
                    onChange={(event) =>
                      setIncludeKubernetes(event.target.checked)
                    }
                  />{" "}
                  Show {kubernetesContainers} Kubernetes-managed containers
                </label>
              )}
              {page === "containers" && (
                <ProjectControls disabled={!inventory || busy} />
              )}
              {inventory ? (
                <ResourceList
                  key={page}
                  page={page}
                  rows={resourceRows}
                  query={query}
                  label={current.label}
                  containers={inventory.containers}
                />
              ) : (
                <Unavailable
                  busy={busy}
                  description={
                    status?.message ??
                    "Could not read the owned engine. Refresh or inspect diagnostics."
                  }
                  onDiagnostics={() => navigate("diagnostics")}
                />
              )}
            </div>
          </section>
        ) : page === "wsl" ? (
          <section className="workspace-page" aria-label={current.label}>
            <Wsl />
          </section>
        ) : page === "pods" ||
          page === "services" ||
          page === "deployments" ||
          page === "persistentVolumeClaims" ? (
          <section className="workspace-page" aria-label={current.label}>
            <Cluster key={page} resource={page} />
          </section>
        ) : (
          <section className="utility-page">
            <header className="utility-heading">
              <h1>{current.label}</h1>
              {page !== "settings" && (
                <button className="button" disabled={busy} onClick={refresh}>
                  <RefreshCw size={14} />
                  Refresh connection
                </button>
              )}
            </header>
            {page === "settings" && (
              <Settings
                tab={settingsTab}
                onTab={setSettingsTab}
                status={status}
                inventory={inventory}
                inventoryBusy={busy}
                settingsRead={settings.read}
                onReloadSettings={() => void settings.reload()}
                theme={theme}
                onTheme={setTheme}
                onMutation={reconcile}
                onOpenWsl={() => navigate("wsl")}
              />
            )}
            {page === "diagnostics" && (
              <>
                <h2>Connection</h2>
                <p>
                  Save a local diagnostic snapshot with the connection result,
                  backend source version, WSL version and distribution names. It
                  excludes raw service logs, client credentials, container
                  environments and workload files.
                </p>
                <button
                  className="button"
                  disabled={diagnosticBusy}
                  onClick={saveSnapshot}
                >
                  Save diagnostic snapshot…
                </button>
                <dl className="properties">
                  <dt>Status</dt>
                  <dd>{status?.message ?? "No connection result"}</dd>
                  {status?.diagnostic && (
                    <>
                      <dt>Detail</dt>
                      <dd>
                        <code>{status.diagnostic}</code>
                      </dd>
                    </>
                  )}
                  <dt>Endpoint</dt>
                  <dd>
                    <code>{backend.endpoint}</code>
                  </dd>
                  <dt>Engine version</dt>
                  <dd>{status?.version ?? "Unavailable"}</dd>
                </dl>
                <p className="notice">
                  Simultaneous-engine coexistence remains unqualified. Native
                  Windows bind paths require the explicit /mnt/ drive form
                  (BL-005).
                </p>
                <h2>WSL host</h2>
                <p>
                  Read installed versions and registered distributions without
                  changing their state.
                </p>
                <button
                  className="button"
                  disabled={diagnosticBusy}
                  onClick={() => void inspect()}
                >
                  {diagnosticBusy ? "Reading host…" : "Read WSL status"}
                </button>
                {diagnostics.map((item) => (
                  <section key={item.name} className="diagnostic-result">
                    <h3>
                      {item.name} ·{" "}
                      {item.success ? "Read successfully" : "Failed"}
                    </h3>
                    <pre>{item.output}</pre>
                  </section>
                ))}
              </>
            )}
            {page === "builds" && <BuildControls disabled={!connected} />}
          </section>
        )}
      </main>
      {palette && (
        <CommandPalette commands={commands} onClose={() => setPalette(false)} />
      )}
    </div>
  );
}
