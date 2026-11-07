import { useEffect, useMemo, useRef, useState } from "react";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { Copy, RefreshCw, Search, Server, Terminal, X } from "lucide-react";
import * as backend from "../backend";
import { useOperation, wslComposeSpec } from "./Operations";
import { copyText } from "./cluster/api";
import {
  openCommand,
  parseDistributions,
  parseVersionInfo,
  type Distro,
  type DistroRole,
} from "./wsl/parse";
import "../styles/cluster.css";

const GROUPS: { role: DistroRole; title: string; note: string }[] = [
  {
    role: "engine",
    title: "OrbWin engine",
    note: "Reserved engine names. Ownership is checked before use.",
  },
  {
    role: "user",
    title: "Your distributions",
    note: "Connect a selected project explicitly.",
  },
  {
    role: "other-engine",
    title: "Other container engines",
    note: "Detected by name. Never modified.",
  },
];
const ROLE_TEXT: Record<DistroRole, string> = {
  engine: "Reserved OrbWin engine name — read-only",
  user: "Your distribution",
  "other-engine": "Other engine — untouched",
};

type Snapshot = {
  distros: Distro[];
  ignored: string[];
  version: { key: string; value: string }[];
  listFailure: string;
};

export function Wsl() {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const search = useRef<HTMLInputElement>(null);
  const rows = useRef<(HTMLButtonElement | null)[]>([]);
  const token = useRef(0);

  async function read() {
    const mine = ++token.current;
    setLoading(true);
    setError("");
    try {
      const results = await backend.diagnostics();
      if (mine !== token.current) return;
      const list = results.find((r) => r.name === "WSL distributions");
      const version = results.find((r) => r.name === "WSL version");
      const parsed = list?.success
        ? parseDistributions(list.output)
        : { distros: [], ignored: [] };
      setSnapshot({
        distros: parsed.distros,
        ignored: parsed.ignored,
        version: version?.success ? parseVersionInfo(version.output) : [],
        listFailure: !list
          ? "The host did not report WSL distributions."
          : list.success
            ? ""
            : list.output,
      });
    } catch (reason) {
      if (mine === token.current) {
        setError(String(reason instanceof Error ? reason.message : reason));
        setSnapshot(null);
      }
    } finally {
      if (mine === token.current) setLoading(false);
    }
  }
  useEffect(() => {
    void read();
    return () => {
      token.current += 1;
    };
  }, []);

  const needle = query.trim().toLowerCase();
  const matches = useMemo(
    () =>
      (snapshot?.distros ?? []).filter(
        (d) =>
          !needle ||
          `${d.name} ${d.state} ${ROLE_TEXT[d.role]}`
            .toLowerCase()
            .includes(needle),
      ),
    [snapshot, needle],
  );
  const visible = GROUPS.flatMap((g) =>
    matches.filter((d) => d.role === g.role),
  );
  const detail = visible.find((d) => d.name === selected) ?? null;

  function body() {
    if (!snapshot) {
      if (loading)
        return (
          <p role="status" className="k8s-state">
            Reading registered WSL distributions…
          </p>
        );
      return (
        <div className="empty-state">
          <Server size={30} />
          <h2>Could not read WSL</h2>
          <p role="alert" className="k8s-error-text">
            {error}
          </p>
          <button className="button" onClick={() => void read()}>
            <RefreshCw size={13} /> Retry
          </button>
        </div>
      );
    }
    if (snapshot.listFailure)
      return (
        <div className="empty-state">
          <Server size={30} />
          <h2>No WSL distributions reported</h2>
          <p>
            WSL returned the message below. If WSL is not installed or has no
            distributions, install it from an administrator terminal; OrbWin
            does not install or change WSL for you.
          </p>
          <p role="alert" className="k8s-error-text">
            {snapshot.listFailure}
          </p>
          <button className="button" onClick={() => void read()}>
            <RefreshCw size={13} /> Retry
          </button>
        </div>
      );
    if (!snapshot.distros.length)
      return (
        <div className="empty-state">
          <Server size={30} />
          <h2>No distributions recognized</h2>
          <p>
            WSL returned output OrbWin could not read as a distribution list.
          </p>
          {snapshot.ignored.length > 0 && (
            <pre className="command-output">{snapshot.ignored.join("\n")}</pre>
          )}
        </div>
      );
    if (!matches.length)
      return (
        <div className="empty-state">
          <Search size={30} />
          <h2>No matching results</h2>
          <p>Try another search or press Escape to clear it.</p>
        </div>
      );
    return GROUPS.map((group) => {
      const items = matches.filter((d) => d.role === group.role);
      if (!items.length) return null;
      return (
        <div key={group.role} role="group" aria-label={group.title}>
          <div className="wsl-group">
            {group.title}
            <small>{group.note}</small>
          </div>
          {items.map((distro) => {
            const at = visible.indexOf(distro);
            const running = distro.state.toLowerCase() === "running";
            return (
              <button
                key={distro.name}
                ref={(element) => {
                  rows.current[at] = element;
                }}
                className={`resource-row ${detail === distro ? "selected" : ""}`}
                aria-pressed={detail === distro}
                onClick={() => setSelected(distro.name)}
                onKeyDown={(event) => {
                  let next = at;
                  if (event.key === "ArrowDown")
                    next = Math.min(at + 1, visible.length - 1);
                  else if (event.key === "ArrowUp") next = Math.max(at - 1, 0);
                  else if (event.key === "Home") next = 0;
                  else if (event.key === "End") next = visible.length - 1;
                  else return;
                  event.preventDefault();
                  setSelected(visible[next].name);
                  rows.current[next]?.focus();
                }}
              >
                <span className="resource-symbol">
                  <Server size={23} />
                  <i
                    className={`status-dot k8s-dot ${running ? "ok" : "idle"}`}
                    title={distro.state}
                  />
                </span>
                <span className="row-copy">
                  <strong>
                    {distro.name}
                    {distro.isDefault && (
                      <span className="wsl-badge">default</span>
                    )}
                  </strong>
                  <small>
                    {distro.state} · WSL {distro.version}
                  </small>
                </span>
              </button>
            );
          })}
        </div>
      );
    });
  }

  return (
    <div
      className="k8s-workspace"
      onKeyDown={(event) => {
        if (
          (event.ctrlKey || event.metaKey) &&
          event.key.toLowerCase() === "f"
        ) {
          event.preventDefault();
          search.current?.focus();
        }
      }}
    >
      <div className="list-pane">
        <header className="list-header">
          <div>
            <h1>WSL distributions</h1>
            <small>
              {snapshot
                ? `${snapshot.distros.length} registered`
                : loading
                  ? "Reading…"
                  : "Not loaded"}
            </small>
          </div>
          <button
            className="icon-button"
            aria-label="Refresh WSL distributions"
            title="Refresh"
            disabled={loading}
            onClick={() => void read()}
          >
            <RefreshCw size={14} />
          </button>
        </header>
        <label className="search">
          <Search size={13} />
          <input
            ref={search}
            value={query}
            placeholder="Search distributions"
            aria-label="Search distributions"
            disabled={!snapshot?.distros.length}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") setQuery("");
            }}
          />
          {query && (
            <button aria-label="Clear search" onClick={() => setQuery("")}>
              <X size={13} />
            </button>
          )}
          <kbd>Ctrl F</kbd>
        </label>
        <div className="resource-list" aria-busy={loading}>
          {body()}
        </div>
      </div>
      <aside className="k8s-inspector" aria-label="Distribution details">
        <ClientTools />
        {detail ? (
          <Detail key={detail.name} distro={detail} onRefresh={read} />
        ) : (
          <div className="detail-content">
            <div className="inspector-empty">
              <Server size={30} />
              <h2>
                {snapshot?.distros.length
                  ? "Select a distribution"
                  : "WSL distributions"}
              </h2>
              <p>
                Registered distributions come from wsl.exe --list --verbose.
              </p>
            </div>
            <Overview snapshot={snapshot} />
          </div>
        )}
      </aside>
    </div>
  );
}

function ClientTools() {
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [failure, setFailure] = useState("");
  const active = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  async function refresh() {
    if (!isTauri() || active.current) return;
    active.current = true;
    setPending(true);
    setMessage("");
    setFailure("");
    try {
      const result = await backend.refreshWslClients();
      if (!mounted.current) return;
      setMessage(
        result.changed
          ? "WSL client tools refreshed."
          : "WSL client tools are already current.",
      );
    } catch (reason) {
      if (mounted.current)
        setFailure(String(reason instanceof Error ? reason.message : reason));
    } finally {
      active.current = false;
      if (mounted.current) setPending(false);
    }
  }
  return (
    <section
      className="wsl-section"
      aria-label="WSL client tools"
      aria-busy={pending}
    >
      <h3>WSL client tools</h3>
      <p className="capability-note">
        Use this after updating OrbWin to refresh its private Docker plugin
        directory. Project connections and registry settings are preserved.
      </p>
      <button
        className="button"
        disabled={pending || !isTauri()}
        onClick={() => void refresh()}
      >
        <RefreshCw size={13} />{" "}
        {pending ? "Refreshing client tools…" : "Refresh client tools"}
      </button>
      {pending && (
        <p role="status" className="capability-note">
          Finishing the local update. Once dispatched, it cannot be cancelled.
        </p>
      )}
      {message && (
        <p role="status" className="capability-note">
          {message}
        </p>
      )}
      {failure && (
        <p role="alert" className="k8s-error-text">
          {failure}
        </p>
      )}
    </section>
  );
}

function Overview({ snapshot }: { snapshot: Snapshot | null }) {
  return (
    <>
      {snapshot && snapshot.version.length > 0 && (
        <section className="wsl-section">
          <h3>WSL version</h3>
          <dl className="detail-properties">
            {snapshot.version.map((item, index) => (
              <div key={`${item.key}:${index}`}>
                <dt>{item.key || "—"}</dt>
                <dd>{item.value}</dd>
              </div>
            ))}
          </dl>
        </section>
      )}
      <p className="capability-note">
        Select a WSL 2 distribution to connect an existing Linux project to
        OrbWin. Creating, importing and deleting distributions use Windows WSL
        tools. OrbWin leaves your shell defaults and other engines unchanged.
      </p>
    </>
  );
}

function CommandLine({ command }: { command: string }) {
  const [note, setNote] = useState("");
  return (
    <div className="wsl-command">
      <code>{command}</code>
      <button
        className="button"
        aria-label={`Copy ${command}`}
        onClick={() =>
          void copyText(command).then((ok) =>
            setNote(ok ? "Copied" : "Clipboard unavailable"),
          )
        }
      >
        <Copy size={13} /> {note || "Copy"}
      </button>
    </div>
  );
}

function Detail({
  distro,
  onRefresh,
}: {
  distro: Distro;
  onRefresh: () => Promise<void>;
}) {
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const canOpen = distro.role === "user" && isTauri();
  return (
    <>
      <header className="inspector-toolbar" />
      <div className="detail-content">
        <div className="detail-title">
          <Server size={30} />
          <div>
            <h2>{distro.name}</h2>
            <p>{ROLE_TEXT[distro.role]}</p>
          </div>
        </div>
        <dl className="detail-properties">
          <div>
            <dt>State</dt>
            <dd>{distro.state}</dd>
          </div>
          <div>
            <dt>WSL version</dt>
            <dd>{distro.version}</dd>
          </div>
          <div>
            <dt>Default distribution</dt>
            <dd>{distro.isDefault ? "Yes" : "No"}</dd>
          </div>
          <div>
            <dt>Managed by</dt>
            <dd>
              {distro.role === "engine"
                ? "Reserved for OrbWin; verified before lifecycle changes"
                : distro.role === "other-engine"
                  ? "Another container engine (by name)"
                  : "You"}
            </dd>
          </div>
        </dl>
        {distro.role === "user" ? (
          <section className="wsl-section">
            <h3>Open</h3>
            <div className="action-row">
              <button
                className="button"
                disabled={!canOpen || busy}
                title={
                  canOpen
                    ? undefined
                    : "Requires the Windows desktop application"
                }
                onClick={() => {
                  setBusy(true);
                  setMessage("");
                  invoke("wsl_open_terminal", { name: distro.name })
                    .then(() => {
                      setMessage("Requested a WSL terminal window.");
                      void onRefresh();
                    })
                    .catch((e) => setMessage(String(e)))
                    .finally(() => setBusy(false));
                }}
              >
                <Terminal size={14} /> Open terminal
              </button>
            </div>
            <p role="status" aria-live="polite">
              {message}
            </p>
            <CommandLine command={openCommand(distro.name)} />
            <small>
              Opening a shell starts the distribution if it is stopped, as wsl
              -d does. OrbWin does not stop, export or edit it.
            </small>
          </section>
        ) : (
          <p className="capability-note">
            {distro.role === "engine"
              ? "This name is reserved for an OrbWin engine guest. Ownership is verified before engine operations. Opening a shell here is disabled so a protected distribution is not altered by accident."
              : "This distribution appears to belong to another container engine. OrbWin leaves it alone so engines can coexist."}
          </p>
        )}
        {distro.role === "user" && distro.version === 2 && (
          <ProjectConnection
            key={distro.name}
            name={distro.name}
            onRefresh={onRefresh}
          />
        )}
        <p className="capability-note">
          Creating or cloning distributions and setting per-distribution
          resource limits are outside this integration. Windows WSL owns those
          capabilities; the shared WSL resource pool is not an OrbWin-only
          limit.
        </p>
      </div>
    </>
  );
}

type ProjectStatus = {
  registration: string;
  export: { source: string; sharedSource: string; signature: string } | null;
  connected: boolean;
  dockerCommand: string;
};
function ProjectConnection({
  name,
  onRefresh,
}: {
  name: string;
  onRefresh: () => Promise<void>;
}) {
  const operation = useOperation();
  const [source, setSource] = useState("");
  const [status, setStatus] = useState<ProjectStatus | null>(null);
  const [stage, setStage] = useState("");
  const [toolMessage, setToolMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  async function act(action: "inspect" | "connect" | "disconnect") {
    setBusy(true);
    setStage(
      action === "connect"
        ? "Connecting project…"
        : action === "disconnect"
          ? "Disconnecting project…"
          : "Checking connection…",
    );
    setError("");
    try {
      const next = await invoke<ProjectStatus>("wsl_project", {
        name,
        action,
        source,
      });
      if (mounted.current) {
        setStatus(next);
        if (next.export) setSource(next.export.source);
        void onRefresh();
      }
    } catch (error) {
      if (mounted.current)
        setError(
          `${String(error)} Refresh connection state before retrying; a requested mount change may have completed.`,
        );
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  async function openProject(tool: "explorer" | "code") {
    const reviewed = status?.export;
    if (!reviewed || busy) return;
    setBusy(true);
    setError("");
    setToolMessage("");
    setStage("Checking the selected project before opening it…");
    try {
      await invoke("open_wsl_project", {
        name,
        registration: status!.registration,
        source: reviewed.source,
        signature: reviewed.signature,
        tool,
      });
      if (mounted.current)
        setToolMessage(
          `Requested ${tool === "code" ? "VS Code" : "Explorer"} for the inspected project. Check that application for completion or prompts.`,
        );
    } catch (reason) {
      if (mounted.current)
        setError(String(reason instanceof Error ? reason.message : reason));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <section className="wsl-section">
      <h3>Connect a Linux project</h3>
      <p>
        Share one existing directory below /home with the owned engine. This
        starts the selected distribution if needed and uses its root mount
        permission. Project files are writable by containers that mount this
        source. Shell defaults stay unchanged. Open a WSL terminal first and
        keep it open while using the project.
      </p>
      <label className="form-field">
        Project directory{" "}
        <input
          aria-label="Linux project directory"
          placeholder="/home/you/project"
          value={source}
          disabled={busy || status?.connected}
          onChange={(event) => {
            setSource(event.target.value);
            setStatus(null);
            setToolMessage("");
          }}
        />
      </label>
      <div className="action-row">
        <button
          className="button"
          disabled={busy || !isTauri()}
          onClick={() => void act("inspect")}
        >
          Refresh connection
        </button>
        <button
          className="button"
          disabled={busy || !source || !isTauri()}
          onClick={() => void act("connect")}
        >
          {status?.connected ? "Verify connection" : "Connect project"}
        </button>
        <button
          className="button"
          disabled={busy || !status?.export || !isTauri()}
          onClick={() => void act("disconnect")}
        >
          Disconnect project
        </button>
      </div>
      <p role="status">
        {busy
          ? stage
          : status
            ? status.connected
              ? "Project export connected."
              : "Project export disconnected."
            : "Refresh to check a previously selected project."}
      </p>
      {error && (
        <p role="alert" className="notice">
          {error}
        </p>
      )}
      {status?.export && (
        <>
          <section>
            <h4>Open this project</h4>
            <p>
              Opens the original Linux files; editor saves affect your project.
              This may start the selected distribution. VS Code needs its WSL
              extension; handle any extension or workspace prompts in the
              editor.
            </p>
            <div className="action-row">
              <button
                className="button"
                disabled={busy || !isTauri()}
                onClick={() => void openProject("explorer")}
              >
                Open in Explorer
              </button>
              <button
                className="button"
                disabled={busy || !isTauri()}
                onClick={() => void openProject("code")}
              >
                Open in VS Code
              </button>
            </div>
            <p role="status">{toolMessage}</p>
          </section>
          {status.connected && (
            <section>
              <h4>Run in this project</h4>
              <div className="action-row">
                {(["validate", "up", "pull", "logs", "down"] as const).map(
                  (action) => (
                    <button
                      key={action}
                      className="button"
                      disabled={busy}
                      onClick={() =>
                        operation(
                          wslComposeSpec(name, status.export!.source, action, {
                            connection: {
                              registration: status.registration,
                              source: status.export!.source,
                              signature: status.export!.signature,
                            },
                          }),
                        )
                      }
                    >
                      Compose {action}…
                    </button>
                  ),
                )}
                <button
                  className="button"
                  disabled={busy}
                  onClick={() =>
                    operation({
                      title: `Build in ${name}`,
                      request: {
                        kind: "wslBuild",
                        distribution: name,
                        connection: {
                          registration: status.registration,
                          source: status.export!.source,
                          signature: status.export!.signature,
                        },
                        directory: "",
                        tag: "",
                      },
                      fields: [
                        {
                          key: "directory",
                          label:
                            "Context directory (optional; selected project root by default)",
                          optional: true,
                          linux: true,
                        },
                        { key: "tag", label: "Image tag" },
                      ],
                      description: `Builds with the pinned Linux Docker client as the distribution's normal user, using an existing context inside ${status.export!.source}. Paths map automatically. Accepted build work may continue after cancellation. No shell defaults or global Docker context are changed.`,
                    })
                  }
                >
                  Build image…
                </button>
              </div>
            </section>
          )}
          <p>
            The project actions above map supported Linux paths automatically.
            For an advanced raw Docker invocation, this is the explicit engine
            bind source; native /home paths belong to the user distribution.
          </p>
          <CommandLine command={status.export.sharedSource} />
        </>
      )}
      {status?.dockerCommand && (
        <>
          <p>
            Docker command for this distribution (append info, run, compose or
            buildx). This targets OrbWin explicitly.
          </p>
          <CommandLine command={status.dockerCommand} />
        </>
      )}
      <small>
        WSL may remove project exports when the distribution exits. Keep a shell
        open while using this project, and reconnect if the export is gone.
        Disconnect unmounts the export without deleting project files. Closing
        OrbWin leaves the engine and project export running.
      </small>
    </section>
  );
}
