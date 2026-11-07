import {
  isValidElement,
  lazy,
  Suspense,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import * as backend from "../../backend";
import { retryBusyRead } from "../../model/read-retry";
import { LocalPortLink } from "../LocalPortLink";
import {
  imageUsers,
  isRunning,
  labels,
  name,
  publishedPorts,
  publicationWarning,
  target,
  text,
  volumeUsers,
  type Page,
  type Row,
} from "./model";

const VolumeSize = lazy(() =>
  import("./VolumeSize").then((module) => ({ default: module.VolumeSize })),
);

type Inspect =
  | { state: "pending" }
  | { state: "ready"; data: Record<string, unknown> }
  | { state: "failed"; message: string };
const kinds = {
  containers: "container",
  images: "image",
  volumes: "volume",
  networks: "network",
} as const;
// Values are returned engine data (rendered as text) or a prepared element.
type Item = [label: string, value: unknown];

function get(data: unknown, ...path: string[]): unknown {
  let value = data;
  for (const part of path)
    value =
      value && typeof value === "object"
        ? (value as Record<string, unknown>)[part]
        : undefined;
  return value;
}
function present(value: unknown) {
  return (
    value !== undefined &&
    value !== null &&
    value !== "" &&
    !(Array.isArray(value) && !value.length)
  );
}
function words(value: unknown) {
  return Array.isArray(value) ? value.map(String).join(" ") : value;
}
function Section({ title, items }: { title: string; items: Item[] }) {
  const shown = items.filter(([, value]) => present(value));
  if (!shown.length) return null;
  return (
    <section className="info-section">
      <h3>{title}</h3>
      <dl className="detail-properties">
        {shown.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{isValidElement(value) ? value : text(value)}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
// Collapsed by default: environment values and labels can carry credentials.
function Pairs({ title, pairs }: { title: string; pairs: [string, string][] }) {
  if (!pairs.length) return null;
  return (
    <details className="info-section info-pairs">
      <summary>
        {title} ({pairs.length})
      </summary>
      <dl className="detail-properties">
        {pairs.map(([key, value], index) => (
          <div key={`${key}:${index}`}>
            <dt>{key}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </details>
  );
}
function env(value: unknown): [string, string][] {
  return Array.isArray(value)
    ? value.map((item) => {
        const raw = String(item);
        const at = raw.indexOf("=");
        return at < 0 ? [raw, ""] : [raw.slice(0, at), raw.slice(at + 1)];
      })
    : [];
}
function record(value: unknown): [string, string][] {
  return value && typeof value === "object" && !Array.isArray(value)
    ? Object.entries(value as Record<string, unknown>).map(([k, v]) => [
        k,
        typeof v === "object" ? JSON.stringify(v) : String(v),
      ])
    : [];
}
function users(rows: Row[] | undefined) {
  if (!rows) return "Container usage was not provided to this view.";
  return rows.length
    ? rows.map(name).join(", ")
    : "No listed container references it.";
}

export function InfoPanel({
  page,
  row,
  containers,
}: {
  page: Page;
  row: Row;
  containers?: Row[];
}) {
  const id = target(page, row);
  const [snapshot, setSnapshot] = useState<{
    page: Page;
    row: Row;
    value: Inspect;
  } | null>(null);
  const inspect: Inspect =
    snapshot?.page === page && snapshot.row === row
      ? snapshot.value
      : { state: "pending" };
  const [retry, setRetry] = useState(0);
  // Inventory rows are immutable snapshots. Re-read on every new snapshot:
  // attachments/configuration can change without changing a list's State.
  // Tag results with their source row so a late old read can never look current.
  useEffect(() => {
    const abort = new AbortController();
    let output = "";
    setSnapshot({ page, row, value: { state: "pending" } });
    retryBusyRead(() => {
      output = "";
      return backend.run(
        crypto.randomUUID(),
        { kind: "inspect", resource: kinds[page], target: id },
        (chunk) => {
          output += chunk;
        },
        abort.signal,
      );
    }, abort.signal)
      .then((outcome) => {
        if (abort.signal.aborted) return;
        if (!outcome.success)
          return setSnapshot({
            page,
            row,
            value: { state: "failed", message: outcome.message },
          });
        const parsed = JSON.parse(output);
        const data = Array.isArray(parsed) ? parsed[0] : parsed;
        if (!data || typeof data !== "object")
          throw new Error("The engine returned no inspect object.");
        setSnapshot({ page, row, value: { state: "ready", data } });
      })
      .catch((error) => {
        if (!abort.signal.aborted)
          setSnapshot({
            page,
            row,
            value: {
              state: "failed",
              message:
                error instanceof SyntaxError
                  ? "Inspect output could not be parsed; it may have exceeded the output limit."
                  : String(error),
            },
          });
      });
    return () => abort.abort();
  }, [page, id, row, retry]);
  const data = inspect.state === "ready" ? inspect.data : undefined;
  const portList = publishedPorts(row);
  const linkable = isRunning(row);
  let body: ReactNode;
  if (page === "containers") {
    const mounts = get(data, "Mounts");
    const networks = get(data, "NetworkSettings", "Networks");
    body = (
      <>
        {publicationWarning(row) && (
          <p className="notice" role="alert">
            {publicationWarning(row)}
          </p>
        )}
        <Section
          title="Status"
          items={[
            ["State", row.State],
            ["Status", row.Status],
            ["Started", isRunning(row) ? get(data, "State", "StartedAt") : ""],
            ["Exit code", isRunning(row) ? "" : get(data, "State", "ExitCode")],
            [
              "Restart policy",
              get(data, "HostConfig", "RestartPolicy", "Name"),
            ],
            ["Created", row.CreatedAt],
          ]}
        />
        <Section
          title="Image"
          items={[
            ["Image", row.Image],
            ["Image ID", get(data, "Image")],
            ["Command", words(get(data, "Config", "Cmd")) ?? row.Command],
            ["Entrypoint", words(get(data, "Config", "Entrypoint"))],
            ["Working directory", get(data, "Config", "WorkingDir")],
          ]}
        />
        <Section
          title="Ports"
          items={portList.map(
            (port): Item => [
              `${port.containerPort}/${port.protocol}`,
              port.url && linkable ? (
                <LocalPortLink port={port.hostPort}>
                  localhost:{port.hostPort}
                </LocalPortLink>
              ) : (
                `${port.hostIp || "*"}:${port.hostPort}`
              ),
            ],
          )}
        />
        <Section
          title="Mounts"
          items={
            Array.isArray(mounts)
              ? mounts.map(
                  (mount, index): Item => [
                    String(get(mount, "Destination") ?? index),
                    `${text(get(mount, "Name") ?? get(mount, "Source"))} (${text(get(mount, "Type"))}${get(mount, "RW") === false ? ", read-only" : ""})`,
                  ],
                )
              : [["Sources", row.Mounts]]
          }
        />
        <Section
          title="Networks"
          items={
            networks && typeof networks === "object"
              ? Object.entries(networks).map(
                  ([key, value]): Item => [
                    key,
                    get(value, "IPAddress") || "No address assigned",
                  ],
                )
              : [["Networks", row.Networks]]
          }
        />
        <Pairs title="Environment" pairs={env(get(data, "Config", "Env"))} />
        <Pairs title="Labels" pairs={Object.entries(labels(row))} />
      </>
    );
  } else if (page === "images") {
    body = (
      <>
        <Section
          title="Image"
          items={[
            ["Repository", row.Repository],
            ["Tag", row.Tag],
            ["ID", row.ID],
            ["Size", row.Size],
            ["Created", row.CreatedSince ?? row.CreatedAt],
            [
              "Platform",
              data &&
                `${text(get(data, "Os"))}/${text(get(data, "Architecture"))}`,
            ],
            ["In use by", users(containers && imageUsers(row, containers))],
          ]}
        />
        <Section
          title="Configuration"
          items={[
            ["Command", words(get(data, "Config", "Cmd"))],
            ["Entrypoint", words(get(data, "Config", "Entrypoint"))],
            [
              "Exposed ports",
              Object.keys(
                (get(data, "Config", "ExposedPorts") as object) ?? {},
              ).join(", "),
            ],
            ["Working directory", get(data, "Config", "WorkingDir")],
          ]}
        />
        <Pairs title="Environment" pairs={env(get(data, "Config", "Env"))} />
        <Pairs title="Labels" pairs={record(get(data, "Config", "Labels"))} />
      </>
    );
  } else if (page === "volumes") {
    body = (
      <>
        <Section
          title="Volume"
          items={[
            ["Name", row.Name],
            ["Driver", row.Driver],
            ["Scope", row.Scope],
            ["Created", get(data, "CreatedAt")],
            ["Mount point", row.Mountpoint],
            [
              "In use by",
              users(containers && volumeUsers(String(row.Name), containers)),
            ],
          ]}
        />
        <Pairs title="Options" pairs={record(get(data, "Options"))} />
        {data &&
          (typeof data.CreatedAt === "string" && data.CreatedAt ? (
            <Suspense
              fallback={<p role="status">Loading volume size controls…</p>}
            >
              <VolumeSize
                key={`${id}:${data.CreatedAt}`}
                name={id}
                createdAt={data.CreatedAt}
              />
            </Suspense>
          ) : (
            <p role="status">
              Volume size refresh requires creation metadata that the engine did
              not return.
            </p>
          ))}
        <Pairs title="Labels" pairs={Object.entries(labels(row))} />
      </>
    );
  } else {
    const ipam = get(data, "IPAM", "Config");
    body = (
      <>
        <Section
          title="Network"
          items={[
            ["Name", row.Name],
            ["Driver", row.Driver],
            ["Scope", row.Scope],
            ["Internal", row.Internal],
            ["IPv6", row.IPv6],
            ...(Array.isArray(ipam)
              ? ipam.flatMap((entry, index): Item[] => [
                  [`Subnet ${index + 1}`, get(entry, "Subnet")],
                  [`Gateway ${index + 1}`, get(entry, "Gateway")],
                ])
              : []),
          ]}
        />
        <Section
          title="Connected containers"
          items={
            data &&
            !Object.keys(
              (get(data, "Containers") as Record<string, unknown>) ?? {},
            ).length
              ? [["Attachments", "No connected containers."]]
              : Object.values(
                  (get(data, "Containers") as Record<string, unknown>) ?? {},
                ).map(
                  (entry): Item => [
                    String(get(entry, "Name")),
                    get(entry, "IPv4Address") || "No address assigned",
                  ],
                )
          }
        />
        <Pairs title="Labels" pairs={Object.entries(labels(row))} />
      </>
    );
  }
  return (
    <div className="info-panel">
      {inspect.state === "pending" && (
        <p role="status" className="info-status">
          Reading full details…
        </p>
      )}
      {inspect.state === "failed" && (
        <p role="status" className="info-status">
          Full details unavailable: {inspect.message} Showing listed fields
          only.{" "}
          <button
            className="button"
            onClick={() => setRetry((value) => value + 1)}
          >
            Retry details
          </button>
        </p>
      )}
      {body}
      <details className="info-section info-pairs">
        <summary>All fields</summary>
        <dl className="detail-properties">
          {Object.entries(row)
            .filter(([, value]) => value !== "" && value !== null)
            .map(([key, value]) => (
              <div key={key}>
                <dt>
                  {key === "ID"
                    ? "ID"
                    : key.replace(/([a-z])([A-Z])/g, "$1 $2")}
                </dt>
                <dd>{text(value)}</dd>
              </div>
            ))}
        </dl>
        {data && (
          <pre className="command-output" aria-label="Full inspect output">
            {JSON.stringify(data, null, 2)}
          </pre>
        )}
      </details>
    </div>
  );
}
