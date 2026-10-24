// Pure helpers over rows returned by the Docker CLI JSON formats. Nothing here
// invents values: unknown or unparsable fields yield undefined/empty results.
export type Row = Record<string, unknown>;
export type Page = "containers" | "images" | "volumes" | "networks";

export function text(value: unknown) {
  return typeof value === "object"
    ? JSON.stringify(value)
    : String(value ?? "—");
}
export function name(row: Row) {
  return text(row.Names ?? row.Repository ?? row.Name ?? row.ID);
}
export function target(page: Page, row: Row) {
  return String(page === "volumes" ? row.Name : row.ID);
}
// Identity used for selection; image tags share an ID, so include the reference.
export function keyOf(page: Page, row: Row) {
  return `${target(page, row)}:${text(row.Repository)}:${text(row.Tag)}`;
}
export function labels(row: Row): Record<string, string> {
  const raw = row.Labels;
  if (raw && typeof raw === "object")
    return Object.fromEntries(
      Object.entries(raw as Record<string, unknown>).map(([k, v]) => [
        k,
        String(v ?? ""),
      ]),
    );
  if (typeof raw !== "string" || !raw) return {};
  // The CLI joins labels with commas without escaping; a segment without "="
  // continues the previous value rather than inventing an empty label.
  const result: Record<string, string> = {};
  let last = "";
  for (const part of raw.split(",")) {
    const at = part.indexOf("=");
    if (at > 0) {
      last = part.slice(0, at);
      result[last] = part.slice(at + 1);
    } else if (last) result[last] += `,${part}`;
  }
  return result;
}
export function project(row: Row) {
  return labels(row)["com.docker.compose.project"] || undefined;
}
// A Compose file is "known" only when the engine recorded exactly one absolute
// Windows path; anything else falls back to per-container actions.
export function composeFile(rows: Row[]) {
  const files = new Set(
    rows.map((row) => labels(row)["com.docker.compose.project.config_files"]),
  );
  if (files.size !== 1) return undefined;
  const [file] = files;
  return file && /^[A-Za-z]:\\[^,]*$/.test(file) ? file : undefined;
}
export function isRunning(row: Row) {
  return String(row.State).toLowerCase() === "running";
}

export type Port = {
  hostIp: string;
  hostPort: number;
  containerPort: string;
  protocol: string;
  url?: string;
};
const loopbackOrAny = new Set(["0.0.0.0", "::", "127.0.0.1", "::1", ""]);
// Parses `docker ps` Ports, e.g. "0.0.0.0:8080->80/tcp, [::]:8080->80/tcp".
// Only TCP ports bound to loopback/any addresses get a localhost URL; the
// protocol spoken on the port is unknown, so callers must not claim HTTP works.
export function ports(value: unknown): Port[] {
  if (typeof value !== "string") return [];
  const seen = new Set<string>();
  const result: Port[] = [];
  for (const part of value.split(/,\s*/)) {
    const match = part
      .trim()
      .match(
        /^(?:\[?([0-9a-fA-F.:]*)\]?:(\d+)(?:-\d+)?->)?(\d+(?:-\d+)?)\/(\w+)$/,
      );
    if (!match || !match[2]) continue;
    const [, hostIp = "", hostPort, containerPort, protocol] = match;
    const key = `${hostPort}/${protocol}`;
    if (seen.has(key)) continue;
    seen.add(key);
    const port = Number(hostPort);
    result.push({
      hostIp,
      hostPort: port,
      containerPort,
      protocol,
      url:
        protocol === "tcp" && loopbackOrAny.has(hostIp) && port > 0
          ? `http://localhost:${port}/`
          : undefined,
    });
  }
  return result;
}

export type SortMode = "status" | "name";
export function compareRows(page: Page, mode: SortMode) {
  return (a: Row, b: Row) => {
    if (mode === "status" && page === "containers") {
      const order = Number(isRunning(b)) - Number(isRunning(a));
      if (order) return order;
    }
    return name(a).localeCompare(name(b), undefined, { numeric: true });
  };
}

export type Entry =
  | { kind: "row"; row: Row }
  | { kind: "group"; project: string; rows: Row[] };
// Compose groups come only from returned project labels. Groups sort among
// standalone rows by the same rule, a group counting as running if any member is.
export function entries(page: Page, rows: Row[], mode: SortMode): Entry[] {
  const compare = compareRows(page, mode);
  const groups = new Map<string, Row[]>();
  const result: Entry[] = [];
  for (const row of rows) {
    const key = page === "containers" ? project(row) : undefined;
    if (!key) result.push({ kind: "row", row });
    else if (groups.has(key)) groups.get(key)!.push(row);
    else {
      const members = [row];
      groups.set(key, members);
      result.push({ kind: "group", project: key, rows: members });
    }
  }
  for (const entry of result)
    if (entry.kind === "group") entry.rows.sort(compare);
  const representative = (entry: Entry): Row =>
    entry.kind === "row"
      ? entry.row
      : {
          Names: entry.project,
          State: entry.rows.some(isRunning) ? "running" : "exited",
        };
  return result.sort((a, b) => compare(representative(a), representative(b)));
}

function list(value: unknown) {
  return typeof value === "string" && value
    ? value.split(",").map((item) => item.trim())
    : [];
}
// `docker ps --no-trunc` Mounts lists named volumes and bind sources by name.
export function volumeUsers(volume: string, containers: Row[]) {
  return containers.filter((row) => list(row.Mounts).includes(volume));
}
export function imageUsers(image: Row, containers: Row[]) {
  const id = String(image.ID ?? "");
  const repository = String(image.Repository ?? "");
  const tag = String(image.Tag ?? "");
  const references = new Set([id, id.replace(/^sha256:/, "")]);
  if (repository && repository !== "<none>" && tag && tag !== "<none>") {
    references.add(`${repository}:${tag}`);
    if (tag === "latest") references.add(repository);
  }
  return containers.filter((row) => references.has(String(row.Image ?? "")));
}

// Stats values such as "12.5%"; undefined when the engine returned none.
export function percent(value: unknown) {
  const parsed = Number.parseFloat(String(value ?? "").replace("%", ""));
  return Number.isFinite(parsed) ? parsed : undefined;
}
// Keeps the newest `limit` samples; the history is in-memory display state only.
export function append<T>(history: T[], sample: T, limit = 60) {
  return [...history, sample].slice(-limit);
}
