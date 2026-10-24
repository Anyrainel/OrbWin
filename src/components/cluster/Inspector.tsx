import {
  lazy,
  Suspense,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Copy, Plug, RefreshCw } from "lucide-react";
import * as backend from "../../backend";
const InteractiveTerminal = lazy(() =>
  import("../Terminal").then((module) => ({ default: module.Terminal })),
);
import { ClusterRemoval, ClusterWait, useOperation } from "../Operations";
import { copyText, kubectl, listObjects, useRead } from "./api";
import {
  ageOf,
  claimHealth,
  deploymentCounts,
  deploymentHealth,
  forwardablePorts,
  health,
  images,
  podCounts,
  podHealth,
  podPorts,
  portText,
  servicePorts,
  serviceType,
  type ClusterKind,
  type Json,
  type K8sObject,
  type PortInfo,
} from "./model";
import { toYaml } from "./yaml";

export const POD_TABS = ["Info", "Logs", "Terminal", "Events", "YAML"] as const;
export const OTHER_TABS = ["Info", "Events", "YAML"] as const;
export type Tab = (typeof POD_TABS)[number];

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}
function Chips({ values }: { values: Record<string, string> | undefined }) {
  const entries = Object.entries(values ?? {});
  if (!entries.length) return <>—</>;
  return (
    <span className="k8s-chips">
      {entries.map(([key, value]) => (
        <code key={key}>
          {key}={value}
        </code>
      ))}
    </span>
  );
}
export function ForwardButton({
  kind,
  object,
  port,
  compact,
}: {
  kind: ClusterKind;
  object: K8sObject;
  port: PortInfo;
  compact?: boolean;
}) {
  const open = useOperation();
  if (kind !== "pods" && kind !== "services") return null;
  const { name, namespace } = object.metadata;
  return (
    <button
      type="button"
      className={compact ? "icon-button k8s-row-action" : "button"}
      aria-label={`Forward port ${port.port} of ${name}`}
      title={`Forward port ${port.port} to localhost`}
      onClick={() =>
        open({
          title: `Forward port ${port.port} of ${name}`,
          request: {
            kind: "clusterForward",
            resource: kind,
            namespace,
            name,
            local: "",
            remote: String(port.port),
          },
          fields: [
            {
              key: "local",
              label: "Local port (0 allocates an available localhost port)",
              placeholder: "0",
            },
          ],
          description:
            "Opens a localhost-only forward in the Port forwards panel. It stays connected while navigating. Stop it from that panel without stopping the workload.",
        })
      }
    >
      <Plug size={compact ? 14 : 13} />
      {!compact && "Forward"}
    </button>
  );
}
function PortList({
  kind,
  object,
  ports,
}: {
  kind: ClusterKind;
  object: K8sObject;
  ports: PortInfo[];
}) {
  const usable = new Set(forwardablePorts(kind, object).map((p) => p.port));
  if (!ports.length) return <>—</>;
  return (
    <ul className="k8s-ports">
      {ports.map((port, index) => (
        <li key={`${port.port}:${port.protocol}:${index}`}>
          <span>
            {port.label && <b>{port.label} </b>}
            {portText(port)}
            {port.target !== undefined && ` → ${port.target}`}
            {port.nodePort !== undefined && ` (node ${port.nodePort})`}
          </span>
          {usable.has(port.port) && port.protocol === "TCP" && (
            <ForwardButton kind={kind} object={object} port={port} />
          )}
        </li>
      ))}
    </ul>
  );
}

function Info({ kind, object }: { kind: ClusterKind; object: K8sObject }) {
  const { metadata } = object;
  const age = (
    <Row label="Age">
      {ageOf(metadata.creationTimestamp)}
      {metadata.creationTimestamp && (
        <small>
          {" "}
          · {new Date(metadata.creationTimestamp).toLocaleString()}
        </small>
      )}
    </Row>
  );
  if (kind === "services") {
    const spec: Json = object.spec ?? {};
    const ingress: Json[] = object.status?.loadBalancer?.ingress ?? [];
    return (
      <dl className="detail-properties">
        <Row label="Type">{serviceType(object)}</Row>
        {age}
        <Row label="Cluster IP">{spec.clusterIP ?? "—"}</Row>
        {ingress.length > 0 && (
          <Row label="Load balancer">
            {ingress.map((i) => i.ip ?? i.hostname).join(", ")}
          </Row>
        )}
        {(spec.externalIPs as string[] | undefined)?.length ? (
          <Row label="External IPs">{spec.externalIPs.join(", ")}</Row>
        ) : null}
        <Row label="Ports">
          <PortList kind={kind} object={object} ports={servicePorts(object)} />
        </Row>
        <Row label="Selector">
          <Chips values={spec.selector} />
        </Row>
        <Row label="Namespace">{metadata.namespace}</Row>
      </dl>
    );
  }
  if (kind === "pods") {
    const { ready, total, restarts } = podCounts(object);
    const containers: Json[] = object.spec?.containers ?? [];
    const statuses: Json[] = object.status?.containerStatuses ?? [];
    return (
      <dl className="detail-properties">
        <Row label="Status">{podHealth(object).label}</Row>
        <Row label="Ready">
          {ready}/{total}
        </Row>
        <Row label="Restarts">{restarts}</Row>
        {age}
        <Row label="Namespace">{metadata.namespace}</Row>
        <Row label="Node">{object.spec?.nodeName ?? "—"}</Row>
        <Row label="Pod IP">{object.status?.podIP ?? "—"}</Row>
        <Row label="Containers">
          <ul className="k8s-ports">
            {containers.map((container) => {
              const status = statuses.find((s) => s.name === container.name);
              return (
                <li key={container.name}>
                  <span>
                    <b>{container.name}</b> {container.image}
                    {status && (
                      <small>
                        {" "}
                        · {status.ready ? "ready" : "not ready"}
                        {status.restartCount
                          ? ` · ${status.restartCount} restarts`
                          : ""}
                      </small>
                    )}
                  </span>
                </li>
              );
            })}
          </ul>
        </Row>
        <Row label="Ports">
          <PortList kind={kind} object={object} ports={podPorts(object)} />
        </Row>
        <Row label="Labels">
          <Chips values={metadata.labels} />
        </Row>
      </dl>
    );
  }
  if (kind === "deployments") {
    const counts = deploymentCounts(object);
    return (
      <dl className="detail-properties">
        <Row label="Status">{deploymentHealth(object).label}</Row>
        <Row label="Ready">
          {counts.ready}/{counts.desired}
        </Row>
        <Row label="Up to date">{counts.updated}</Row>
        <Row label="Available">{counts.available}</Row>
        {age}
        <Row label="Namespace">{metadata.namespace}</Row>
        <Row label="Strategy">{object.spec?.strategy?.type ?? "—"}</Row>
        <Row label="Images">{images(object).join(", ") || "—"}</Row>
        <Row label="Selector">
          <Chips values={object.spec?.selector?.matchLabels} />
        </Row>
        <Row label="Labels">
          <Chips values={metadata.labels} />
        </Row>
      </dl>
    );
  }
  const spec: Json = object.spec ?? {};
  return (
    <dl className="detail-properties">
      <Row label="Status">{claimHealth(object).label}</Row>
      <Row label="Capacity">
        {object.status?.capacity?.storage ??
          spec.resources?.requests?.storage ??
          "—"}
      </Row>
      <Row label="Requested">{spec.resources?.requests?.storage ?? "—"}</Row>
      <Row label="Storage class">{spec.storageClassName ?? "—"}</Row>
      <Row label="Access modes">
        {(spec.accessModes as string[] | undefined)?.join(", ") || "—"}
      </Row>
      <Row label="Volume">{spec.volumeName ?? "—"}</Row>
      {age}
      <Row label="Namespace">{metadata.namespace}</Row>
      <Row label="Labels">
        <Chips values={metadata.labels} />
      </Row>
    </dl>
  );
}

function Loading({ what }: { what: string }) {
  return (
    <p role="status" className="k8s-state">
      Loading {what}…
    </p>
  );
}
function Failure({ message, retry }: { message: string; retry: () => void }) {
  return (
    <div className="k8s-state">
      <p role="alert" className="notice">
        {message}
      </p>
      <button className="button" onClick={retry}>
        <RefreshCw size={13} /> Retry
      </button>
    </div>
  );
}

function Logs({ object }: { object: K8sObject }) {
  const { name, namespace } = object.metadata;
  const [follow, setFollow] = useState(true);
  const [nonce, setNonce] = useState(0);
  const [log, setLog] = useState({ text: "", status: "loading", error: "" });
  const reader = useRef<AbortController | null>(null);
  const pinned = useRef(true);
  useEffect(() => {
    const controller = new AbortController();
    reader.current = controller;
    setLog({ text: "", status: "loading", error: "" });
    pinned.current = true;
    void backend
      .run(
        crypto.randomUUID(),
        { kind: "clusterLogs", namespace, name, follow },
        (chunk) => {
          if (!controller.signal.aborted)
            setLog((previous) => ({
              ...previous,
              text: (previous.text + chunk).slice(-131072),
            }));
        },
        controller.signal,
      )
      .then((outcome) => {
        if (!controller.signal.aborted)
          setLog((previous) => ({
            ...previous,
            status: outcome.success ? "done" : "error",
            error: outcome.success ? "" : outcome.message,
          }));
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setLog((previous) => ({
            ...previous,
            status: "error",
            error: String(error),
          }));
      });
    return () => controller.abort();
  }, [object.metadata.uid, namespace, name, follow, nonce]);
  const reload = () => setNonce((value) => value + 1);
  const [filter, setFilter] = useState("");
  const [wrap, setWrap] = useState(false);
  const [note, setNote] = useState("");
  const box = useRef<HTMLPreElement>(null);
  const shown = useMemo(() => {
    const needle = filter.toLowerCase();
    return needle
      ? log.text
          .split("\n")
          .filter((line) => line.toLowerCase().includes(needle))
          .join("\n")
      : log.text;
  }, [log.text, filter]);
  useEffect(() => {
    const element = box.current;
    if (element && pinned.current) element.scrollTop = element.scrollHeight;
  }, [shown]);
  return (
    <div className="k8s-pane">
      <div className="k8s-toolbar">
        <label className="search k8s-inline-search">
          <input
            value={filter}
            aria-label="Filter log lines"
            placeholder="Filter log lines"
            onChange={(e) => setFilter(e.target.value)}
          />
        </label>
        <label>
          <input
            type="checkbox"
            checked={wrap}
            onChange={(e) => setWrap(e.target.checked)}
          />{" "}
          Wrap
        </label>
        <button className="button" onClick={reload}>
          <RefreshCw size={13} /> Refresh
        </button>
        {log.status === "loading" && (
          <button
            className="button"
            onClick={() => {
              reader.current?.abort();
              setLog((previous) => ({ ...previous, status: "paused" }));
            }}
          >
            Pause read
          </button>
        )}
        <label>
          <input
            type="checkbox"
            checked={follow}
            onChange={(event) => setFollow(event.target.checked)}
          />{" "}
          Follow
        </label>
        <button
          className="button"
          disabled={!log.text}
          onClick={() =>
            void copyText(shown).then((ok) =>
              setNote(ok ? "Copied." : "Clipboard unavailable."),
            )
          }
        >
          <Copy size={13} /> Copy
        </button>
        <button
          className="button"
          disabled={!log.text}
          onClick={() =>
            void backend
              .exportText(shown)
              .then((saved) => saved && setNote("Saved."))
              .catch((e) => setNote(String(e)))
          }
        >
          Save…
        </button>
        <span role="status" aria-live="polite">
          {note}
        </span>
      </div>
      <small>
        Last 500 lines per container, with source and timestamps. The view
        retains the latest 128 KiB. Follow ends after one hour or the native
        output limit; refresh to reconnect. Scroll up to read without jumping to
        new output.
      </small>
      {log.status === "loading" && (
        <>
          <p role="status">
            {follow ? "Following pod output…" : "Reading logs…"}
          </p>
        </>
      )}
      {log.status === "paused" && (
        <p role="status">
          Read paused. Retained output is available below; refresh to reconnect.
        </p>
      )}
      {log.status === "error" && <Failure message={log.error} retry={reload} />}
      {(log.text !== "" || log.status === "done") &&
        (log.text.trim() === "" ? (
          <p className="k8s-state">The pod has not written any log output.</p>
        ) : shown === "" ? (
          <p className="k8s-state">No log lines match the filter.</p>
        ) : (
          <pre
            ref={box}
            tabIndex={0}
            aria-label="Pod logs"
            onScroll={(event) => {
              const element = event.currentTarget;
              pinned.current =
                element.scrollHeight -
                  element.scrollTop -
                  element.clientHeight <
                40;
            }}
            className={`command-output k8s-log ${wrap ? "wrap" : ""}`}
          >
            {shown}
          </pre>
        ))}
    </div>
  );
}

function Terminal({ object }: { object: K8sObject }) {
  const containers = (object.spec?.containers ?? []) as { name: string }[];
  const [selected, setSelected] = useState(containers[0]?.name ?? "");
  const container = containers.some((item) => item.name === selected)
    ? selected
    : (containers[0]?.name ?? "");
  if (!container)
    return (
      <p className="notice">
        The selected pod has no regular container to open.
      </p>
    );
  return (
    <div className="k8s-pane">
      <label>
        Container{" "}
        <select
          aria-label="Pod container"
          value={container}
          onChange={(event) => setSelected(event.target.value)}
        >
          {containers.map((item) => (
            <option key={item.name}>{item.name}</option>
          ))}
        </select>
      </label>
      <Suspense fallback={<p role="status">Loading terminal…</p>}>
        <InteractiveTerminal
          key={object.metadata.uid + ":" + container}
          target={object.metadata.uid}
          pod={{
            namespace: object.metadata.namespace,
            name: object.metadata.name,
            uid: object.metadata.uid,
            container,
          }}
        />
      </Suspense>
    </div>
  );
}

function Events({ object }: { object: K8sObject }) {
  const { namespace, uid } = object.metadata;
  const events = useRead(
    async (signal) => {
      const all = await listObjects("events", namespace, signal);
      return JSON.stringify(
        all
          .filter((event) => (event as Json).involvedObject?.uid === uid)
          .sort((a, b) => stamp(b) - stamp(a)),
      );
    },
    uid,
    true,
  );
  const rows: Json[] = useMemo(() => {
    try {
      return events.text ? JSON.parse(events.text) : [];
    } catch {
      return [];
    }
  }, [events.text]);
  return (
    <div className="k8s-pane">
      <div className="k8s-toolbar">
        <button
          className="button"
          disabled={events.status === "loading"}
          onClick={events.reload}
        >
          <RefreshCw size={13} /> Refresh
        </button>
      </div>
      {events.status === "loading" && <Loading what="events" />}
      {events.status === "error" && (
        <Failure message={events.error} retry={events.reload} />
      )}
      {events.status === "done" &&
        (rows.length === 0 ? (
          <p className="k8s-state">
            The cluster returned no events for this object. Kubernetes expires
            events after about an hour.
          </p>
        ) : (
          <ul className="k8s-events">
            {rows.map((event) => (
              <li key={event.metadata.uid}>
                <span className={`k8s-event-type ${event.type}`}>
                  {event.type ?? "Normal"}
                </span>
                <strong>{event.reason}</strong>
                <span>{event.message}</span>
                <small>
                  {ageOf(
                    event.lastTimestamp ??
                      event.eventTime ??
                      event.metadata.creationTimestamp,
                  )}
                  {event.count > 1 ? ` · ×${event.count}` : ""}
                </small>
              </li>
            ))}
          </ul>
        ))}
    </div>
  );
}
function stamp(event: K8sObject): number {
  const e = event as Json;
  return (
    Date.parse(
      e.lastTimestamp ?? e.eventTime ?? e.metadata.creationTimestamp ?? "",
    ) || 0
  );
}

function Yaml({ object }: { object: K8sObject }) {
  const text = useMemo(() => toYaml(object), [object]);
  const [note, setNote] = useState("");
  return (
    <div className="k8s-pane">
      <div className="k8s-toolbar">
        <button
          className="button"
          onClick={() =>
            void copyText(text).then((ok) =>
              setNote(ok ? "Copied." : "Clipboard unavailable."),
            )
          }
        >
          <Copy size={13} /> Copy
        </button>
        <span role="status" aria-live="polite">
          {note}
        </span>
      </div>
      <small>
        Rendered from the object returned by the last cluster read; refresh the
        list to update it.
      </small>
      <pre
        tabIndex={0}
        aria-label="Resource YAML"
        className="command-output k8s-yaml"
      >
        {text}
      </pre>
    </div>
  );
}

export function Inspector({
  kind,
  object,
  tab,
  onTab,
  icon,
}: {
  kind: ClusterKind;
  object: K8sObject;
  tab: Tab;
  onTab: (tab: Tab) => void;
  icon: ReactNode;
}) {
  const tabs: readonly Tab[] = kind === "pods" ? POD_TABS : OTHER_TABS;
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const status = health(kind, object);
  const { metadata } = object;
  return (
    <>
      <header className="inspector-toolbar">
        <div
          className="tabs"
          role="tablist"
          aria-label="Resource views"
          onKeyDown={(event) => {
            const index = tabs.indexOf(tab);
            let next = index;
            if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
            else if (event.key === "ArrowLeft")
              next = (index - 1 + tabs.length) % tabs.length;
            else if (event.key === "Home") next = 0;
            else if (event.key === "End") next = tabs.length - 1;
            else return;
            event.preventDefault();
            onTab(tabs[next]);
            refs.current[next]?.focus();
          }}
        >
          {tabs.map((item, index) => (
            <button
              key={item}
              ref={(element) => {
                refs.current[index] = element;
              }}
              id={`k8s-tab-${item}`}
              role="tab"
              tabIndex={tab === item ? 0 : -1}
              aria-selected={tab === item}
              aria-controls="k8s-detail"
              onClick={() => onTab(item)}
            >
              {item}
            </button>
          ))}
        </div>
      </header>
      <div
        id="k8s-detail"
        role="tabpanel"
        aria-labelledby={`k8s-tab-${tab}`}
        className="detail-content"
      >
        <div className="detail-title">
          {icon}
          <div>
            <h2>{metadata.name}</h2>
            <p>
              {metadata.namespace} ·{" "}
              <span className={`k8s-tone ${status.tone}`}>{status.label}</span>
            </p>
          </div>
          <div className="k8s-title-actions">
            <ClusterWait resource={kind} metadata={metadata} />
            <ClusterRemoval resource={kind} metadata={metadata} />
          </div>
        </div>
        {tab === "Info" && <Info kind={kind} object={object} />}
        {tab === "Logs" && kind === "pods" && <Logs object={object} />}
        {tab === "Terminal" && kind === "pods" && <Terminal object={object} />}
        {tab === "Events" && <Events object={object} />}
        {tab === "YAML" && <Yaml object={object} />}
      </div>
    </>
  );
}
