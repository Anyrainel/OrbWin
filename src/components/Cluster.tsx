import { useEffect, useMemo, useRef, useState } from "react";
import {
  Boxes,
  ChevronDown,
  ChevronRight,
  Database,
  HardDrive,
  Layers,
  Network,
  RefreshCw,
  Search,
  Trash2,
  X,
} from "lucide-react";
import * as backend from "../backend";
import {
  ClusterControls,
  ClusterManifest,
  ClusterRemoval,
} from "./Operations";
import { CANCELLED, listObjects } from "./cluster/api";
import { ForwardButton, Inspector, type Tab } from "./cluster/Inspector";
import {
  ageOf,
  classifyReadError,
  forwardablePorts,
  groupByNamespace,
  health,
  namespacesOf,
  searchText,
  summary,
  type ClusterKind,
  type K8sObject,
  type ReadFailure,
} from "./cluster/model";
import "../styles/cluster.css";

const LABELS: Record<ClusterKind, { plural: string; singular: string }> = {
  pods: { plural: "Pods", singular: "pod" },
  services: { plural: "Services", singular: "service" },
  deployments: { plural: "Deployments", singular: "deployment" },
  persistentVolumeClaims: {
    plural: "Storage claims",
    singular: "storage claim",
  },
};
function KindIcon({ kind, size }: { kind: ClusterKind; size: number }) {
  const Icon =
    kind === "pods"
      ? Boxes
      : kind === "services"
        ? Network
        : kind === "deployments"
          ? Layers
          : HardDrive;
  return <Icon size={size} />;
}

export function Cluster({
  resource,
  onOpenSettings,
}: {
  resource: backend.ClusterResource;
  onOpenSettings?: () => void;
}) {
  const kind = resource as ClusterKind;
  const label = LABELS[kind] ?? LABELS.pods;
  const [objects, setObjects] = useState<K8sObject[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [failure, setFailure] = useState<{
    type: ReadFailure | "cancelled";
    message: string;
  } | null>(null);
  const [query, setQuery] = useState("");
  const [namespace, setNamespace] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("Info");
  const search = useRef<HTMLInputElement>(null);
  const rows = useRef<(HTMLButtonElement | null)[]>([]);
  const active = useRef<AbortController | null>(null);
  const queued = useRef(false);
  const mounted = useRef(false);

  async function read(silent = false) {
    if (active.current) {
      queued.current = true;
      return;
    }
    const abort = new AbortController();
    active.current = abort;
    if (!silent) setLoading(true);
    try {
      const items = await listObjects(kind, "", abort.signal);
      if (abort.signal.aborted) return;
      setObjects(items);
      setFailure(null);
    } catch (error) {
      if (abort.signal.aborted && !mounted.current) return;
      const message = error instanceof Error ? error.message : String(error);
      if (abort.signal.aborted)
        setFailure({ type: "cancelled", message: CANCELLED });
      else setFailure({ type: classifyReadError(message), message });
    } finally {
      if (active.current === abort) active.current = null;
      if (mounted.current) {
        setLoading(false);
        // Reconcile mutations that finished while an older read was in flight.
        if (queued.current) {
          queued.current = false;
          void readRef.current(true);
        }
      }
    }
  }
  const readRef = useRef(read);
  readRef.current = read;

  useEffect(() => {
    mounted.current = true;
    void readRef.current();
    const changed = () => void readRef.current(true);
    window.addEventListener("orbwin:cluster-changed", changed);
    // Light background refresh keeps phases and ready counts current while the
    // page is visible. It is read-only and skipped while a read is in flight.
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible" && !active.current)
        void readRef.current(true);
    }, 15000);
    return () => {
      mounted.current = false;
      queued.current = false;
      active.current?.abort();
      window.clearInterval(timer);
      window.removeEventListener("orbwin:cluster-changed", changed);
    };
  }, []);

  const namespaces = useMemo(() => namespacesOf(objects ?? []), [objects]);
  // A selected namespace that no longer has objects must not hide everything.
  const scope = namespaces.includes(namespace) ? namespace : "";
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return (objects ?? []).filter(
      (object) =>
        (!scope || object.metadata.namespace === scope) &&
        (!needle || searchText(kind, object).includes(needle)),
    );
  }, [objects, query, scope, kind]);
  const groups = useMemo(() => groupByNamespace(filtered), [filtered]);
  const visible = groups.flatMap(([ns, items]) =>
    collapsed.has(ns) ? [] : items,
  );
  // Selection follows the current snapshot by UID; a deleted object disappears.
  const detail = filtered.find((o) => o.metadata.uid === selected) ?? null;
  const now = Date.now();

  function select(object: K8sObject) {
    if (object.metadata.uid !== selected) setTab("Info");
    setSelected(object.metadata.uid);
  }
  function toggle(ns: string) {
    setCollapsed((previous) => {
      const next = new Set(previous);
      if (next.has(ns)) next.delete(ns);
      else next.add(ns);
      return next;
    });
  }

  function body() {
    if (objects === null) {
      if (loading)
        return (
          <div className="k8s-state">
            <p role="status">
              Reading {label.plural.toLowerCase()} from the owned cluster…
            </p>
            <button className="button" onClick={() => active.current?.abort()}>
              Cancel read
            </button>
          </div>
        );
      if (failure?.type === "disabled")
        return (
          <div className="empty-state">
            <Boxes size={30} />
            <h2>Kubernetes is not enabled</h2>
            <p>
              The OrbWin workload cluster has no kubeconfig yet. It is either
              disabled or its setup has not finished. Docker workflows do not
              need it. Enabling it can download components and take several
              minutes.
            </p>
            <ClusterControls only="enable" />
            {onOpenSettings && (
              <button className="button" onClick={onOpenSettings}>
                Open Settings
              </button>
            )}
            <button className="button" onClick={() => void read()}>
              <RefreshCw size={13} /> Check again
            </button>
          </div>
        );
      return (
        <div className="empty-state">
          <Boxes size={30} />
          <h2>
            {failure?.type === "preview"
              ? "Desktop app required"
              : failure?.type === "cancelled"
                ? "Read cancelled"
                : `Could not read ${label.plural.toLowerCase()}`}
          </h2>
          <p role="alert" className="k8s-error-text">
            {failure?.message}
          </p>
          <button className="button" onClick={() => void read()}>
            <RefreshCw size={13} /> Retry
          </button>
        </div>
      );
    }
    if (objects.length === 0)
      return (
        <div className="empty-state">
          <KindIcon kind={kind} size={30} />
          <h2>No {label.plural.toLowerCase()} yet</h2>
          <p>
            The cluster returned no {label.plural.toLowerCase()} in any
            namespace. Apply a manifest to create some.
          </p>
          <ClusterManifest />
        </div>
      );
    if (filtered.length === 0)
      return (
        <div className="empty-state">
          <Search size={30} />
          <h2>No matching results</h2>
          <p>Try another search or namespace, or press Escape to clear it.</p>
        </div>
      );
    let index = -1;
    return groups.map(([ns, items]) => (
      <div key={ns}>
        <button
          className="project-row"
          aria-expanded={!collapsed.has(ns)}
          onClick={() => toggle(ns)}
        >
          {collapsed.has(ns) ? (
            <ChevronRight size={14} />
          ) : (
            <ChevronDown size={14} />
          )}
          <Database size={17} />
          <strong>{ns}</strong>
          <small>{items.length}</small>
        </button>
        {!collapsed.has(ns) &&
          items.map((object) => {
            index += 1;
            const at = index;
            const status = health(kind, object);
            const ports = forwardablePorts(kind, object);
            const isSelected = detail === object;
            return (
              <div key={object.metadata.uid} className="k8s-row">
                <button
                  ref={(element) => {
                    rows.current[at] = element;
                  }}
                  className={`resource-row in-project ${isSelected ? "selected" : ""}`}
                  aria-pressed={isSelected}
                  onClick={() => select(object)}
                  onKeyDown={(event) => {
                    let next = at;
                    if (event.key === "ArrowDown")
                      next = Math.min(at + 1, visible.length - 1);
                    else if (event.key === "ArrowUp") next = Math.max(at - 1, 0);
                    else if (event.key === "Home") next = 0;
                    else if (event.key === "End") next = visible.length - 1;
                    else return;
                    event.preventDefault();
                    select(visible[next]);
                    rows.current[next]?.focus();
                  }}
                >
                  <span className="resource-symbol">
                    <KindIcon kind={kind} size={23} />
                    <i
                      className={`status-dot k8s-dot ${status.tone}`}
                      title={status.label}
                    />
                  </span>
                  <span className="row-copy">
                    <strong>{object.metadata.name}</strong>
                    <small>{summary(kind, object)}</small>
                  </span>
                  <span className="row-meta">
                    {ageOf(object.metadata.creationTimestamp, now)}
                  </span>
                </button>
                <span className="k8s-row-actions">
                  {ports.length === 1 && (
                    <ForwardButton
                      kind={kind}
                      object={object}
                      port={ports[0]}
                      compact
                    />
                  )}
                  <ClusterRemoval
                    resource={kind}
                    metadata={object.metadata}
                    className="icon-button k8s-row-action"
                    label={<Trash2 size={14} />}
                    ariaLabel={`Delete ${object.metadata.name}`}
                  />
                </span>
              </div>
            );
          })}
      </div>
    ));
  }

  return (
    <div
      className="k8s-workspace"
      onKeyDown={(event) => {
        if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
          event.preventDefault();
          search.current?.focus();
          search.current?.select();
        }
      }}
    >
      <div className="list-pane" aria-label={`${label.plural} list`}>
        <header className="list-header">
          <div>
            <h1>{label.plural}</h1>
            <small>
              {objects === null
                ? loading
                  ? "Reading…"
                  : "Not loaded"
                : `${filtered.length} of ${objects.length}`}
            </small>
          </div>
          {loading && objects !== null ? (
            <button
              className="icon-button"
              aria-label="Cancel cluster read"
              title="Cancel read"
              onClick={() => active.current?.abort()}
            >
              <X size={14} />
            </button>
          ) : (
            <button
              className="icon-button"
              aria-label="Refresh cluster"
              title="Refresh"
              disabled={loading}
              onClick={() => void read()}
            >
              <RefreshCw size={14} />
            </button>
          )}
        </header>
        <label className="search">
          <Search size={13} />
          <input
            ref={search}
            value={query}
            placeholder={`Search ${label.plural.toLowerCase()}`}
            aria-label={`Search ${label.plural.toLowerCase()}`}
            disabled={!objects}
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
        {objects && namespaces.length > 1 && (
          <label className="k8s-filter">
            Namespace
            <select
              value={scope}
              onChange={(e) => setNamespace(e.target.value)}
            >
              <option value="">All namespaces</option>
              {namespaces.map((ns) => (
                <option key={ns} value={ns}>
                  {ns}
                </option>
              ))}
            </select>
          </label>
        )}
        {failure && objects !== null && (
          <p role="alert" className="k8s-stale">
            {failure.type === "cancelled"
              ? failure.message
              : `Last refresh failed; showing earlier data. ${failure.message}`}
          </p>
        )}
        <div
          className="resource-list"
          aria-label={`${label.plural} in the owned cluster`}
          aria-busy={loading}
        >
          {body()}
        </div>
        {(objects !== null || failure?.type === "disabled") && (
          <details className="project-controls">
            <summary>Cluster</summary>
            <ClusterControls />
            <ClusterManifest />
          </details>
        )}
      </div>
      <aside className="k8s-inspector" aria-label={`${label.singular} details`}>
        {detail ? (
          <Inspector
            key={detail.metadata.uid}
            kind={kind}
            object={detail}
            tab={tab}
            onTab={setTab}
            icon={<KindIcon kind={kind} size={30} />}
          />
        ) : (
          <div className="inspector-empty">
            <KindIcon kind={kind} size={30} />
            <h2>
              {objects === null
                ? "No cluster data"
                : filtered.length
                  ? `Select a ${label.singular}`
                  : "Nothing to inspect"}
            </h2>
            <p>
              {objects === null
                ? "Details appear after the cluster is read."
                : "Details come from the last cluster read."}
            </p>
          </div>
        )}
      </aside>
    </div>
  );
}
