import { useEffect, useMemo, useRef, useState } from "react";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { Copy, RefreshCw, Search, Server, Terminal, X } from "lucide-react";
import * as backend from "../backend";
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
    note: "Managed by OrbWin. Read-only here.",
  },
  {
    role: "user",
    title: "Your distributions",
    note: "Untouched by OrbWin.",
  },
  {
    role: "other-engine",
    title: "Other container engines",
    note: "Detected by name. Never modified.",
  },
];
const ROLE_TEXT: Record<DistroRole, string> = {
  engine: "OrbWin engine — read-only",
  user: "Your distribution — untouched",
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
  const visible = GROUPS.flatMap((g) => matches.filter((d) => d.role === g.role));
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
            distributions, install it from an administrator terminal; OrbWin does
            not install or change WSL for you.
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
                    {distro.isDefault && <span className="wsl-badge">default</span>}
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
        if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
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
        {detail ? (
          <Detail key={detail.name} distro={detail} />
        ) : (
          <div className="detail-content">
            <div className="inspector-empty">
              <Server size={30} />
              <h2>
                {snapshot?.distros.length
                  ? "Select a distribution"
                  : "WSL distributions"}
              </h2>
              <p>Registered distributions come from wsl.exe --list --verbose.</p>
            </div>
            <Overview snapshot={snapshot} />
          </div>
        )}
      </aside>
    </div>
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
        Creating, importing, deleting and sharing files with distributions is
        not available yet. OrbWin lists what WSL reports and never changes your
        distributions. Use wsl --install or wsl --import in a terminal for now.
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

function Detail({ distro }: { distro: Distro }) {
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
                ? "OrbWin (engine guest)"
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
                  canOpen ? undefined : "Requires the Windows desktop application"
                }
                onClick={() => {
                  setBusy(true);
                  setMessage("");
                  invoke("wsl_open_terminal", { name: distro.name })
                    .then(() => setMessage("Opened a terminal window."))
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
              Opening a shell starts the distribution if it is stopped, as
              wsl -d does. OrbWin does not stop, export or edit it.
            </small>
          </section>
        ) : (
          <p className="capability-note">
            {distro.role === "engine"
              ? "This is OrbWin's engine guest. Start and stop it from Settings. Opening a shell here is disabled so the engine's state is not altered by accident."
              : "This distribution appears to belong to another container engine. OrbWin leaves it alone so engines can coexist."}
          </p>
        )}
        <p className="capability-note">
          Per-distribution integration (file sharing, creating or cloning
          distributions, resource limits) is not available yet and is not
          simulated.
        </p>
      </div>
    </>
  );
}
