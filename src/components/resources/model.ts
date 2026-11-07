// Pure helpers over rows returned by the Docker CLI JSON formats. Nothing here
// invents values: unknown or unparsable fields yield undefined/empty results.
export type Row = Record<string, unknown>;
export type Page = "containers" | "images" | "volumes" | "networks";

// Snapshot replacement/cancel invalidates both success and failure before
// parsing, even if a client completes late. No retry or engine mutation.
export async function readResourceMetadata<T>(
  run: (
    output: (chunk: string, stream?: "stdout" | "stderr") => void,
    signal: AbortSignal,
  ) => Promise<{ success: boolean; message: string }>,
  signal: AbortSignal,
  isCurrent: () => boolean,
  parse: (output: string) => T,
): Promise<T | undefined> {
  const current = () => !signal.aborted && isCurrent();
  if (!current()) return;
  let output = "",
    diagnostic = "",
    overflow = false;
  try {
    const outcome = await run((chunk, stream) => {
      if (stream === "stderr") diagnostic = (diagnostic + chunk).slice(0, 8192);
      else if (output.length + chunk.length > 1024 * 1024) overflow = true;
      else output += chunk;
    }, signal);
    if (!current()) return;
    if (!outcome.success) throw new Error(`${outcome.message} ${diagnostic}`);
    if (overflow)
      throw new Error("Resource metadata exceeded its display limit.");
    return parse(output);
  } catch (error) {
    if (current()) throw error;
  }
}

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
// Labels are hints for an editable review, never permission to run a project.
// Compose encodes config_files as an unescaped comma list: reject ambiguous
// relative fragments, control characters, or inconsistent snapshots.
export function composeInputs(projectName: string, rows: Row[]) {
  const absolute = (value: string) =>
    /^[A-Za-z]:[\\/]/.test(value) &&
    !/[\r\n\0,=]/.test(value) &&
    value.length <= 32767;
  if (!rows.length || rows.some((row) => project(row) !== projectName)) return;
  const snapshots = rows.map((row) => {
    const data = labels(row);
    return {
      files: data["com.docker.compose.project.config_files"] ?? "",
      directory: data["com.docker.compose.project.working_dir"] ?? "",
      env: data["com.docker.compose.project.environment_file"] ?? "",
    };
  });
  const first = snapshots[0];
  if (snapshots.some((item) => JSON.stringify(item) !== JSON.stringify(first)))
    return;
  // Our Windows adapter canonicalizes paths before dispatch; Compose records
  // the Win32 verbatim drive prefix. Strip only that prefix, never UNC/devices.
  const drivePath = (value: string) =>
    value.startsWith("\\\\?\\") ? value.slice(4) : value;
  const files = first.files.split(",").map(drivePath);
  const directory = drivePath(first.directory);
  if (
    files.length > 16 ||
    !files.every(absolute) ||
    !absolute(directory) ||
    first.env
  )
    return;
  return {
    project: projectName,
    file: files[0],
    overrides: files.slice(1),
    profiles: [] as string[],
    directory,
  };
}
export function isRunning(row: Row) {
  return String(row.State).toLowerCase() === "running";
}
export function isPaused(row: Row) {
  return String(row.State).toLowerCase() === "paused";
}

export type WslComposeInputs = {
  signature?: string;
  distribution: string;
  source: string;
  file: string;
  overrides: string[];
  envFiles?: string[];
  directory: string;
  project: string;
  profiles: string[];
  services?: string[];
};
export function wslComposeInputs(
  projectName: string,
  rows: Row[],
): WslComposeInputs | undefined {
  if (!rows.length || rows.some((row) => project(row) !== projectName)) return;
  const values = rows.map(
    (row) => labels(row)["dev.orbwin.wsl.compose-inputs"],
  );
  if (
    !values[0] ||
    values[0].length > 32768 ||
    values.some((value) => value !== values[0])
  )
    return;
  try {
    const item = JSON.parse(values[0]) as WslComposeInputs;
    const text = (value: unknown): value is string =>
      typeof value === "string" &&
      value.length <= 4096 &&
      !/[\r\n\0\\]/.test(value);
    const list = (value: unknown, max: number) =>
      Array.isArray(value) && value.length <= max && value.every(text);
    if (
      !text(item.distribution) ||
      !item.distribution ||
      !text(item.source) ||
      !item.source.startsWith("/home/") ||
      !text(item.file) ||
      !item.file ||
      !text(item.directory) ||
      item.project !== projectName ||
      !list(item.overrides, 15) ||
      !list(item.envFiles ?? [], 16) ||
      !list(item.profiles, 32) ||
      (item.signature !== undefined &&
        (typeof item.signature !== "string" ||
          item.signature.length > 64 ||
          !/^[0-9]+:[0-9]+$/.test(item.signature)))
    )
      return;
    return {
      distribution: item.distribution,
      source: item.source,
      file: item.file,
      overrides: item.overrides,
      ...(item.envFiles ? { envFiles: item.envFiles } : {}),
      directory: item.directory,
      project: projectName,
      profiles: item.profiles,
      ...(item.signature ? { signature: item.signature } : {}),
    };
  } catch {
    return;
  }
}
export function composeServiceInputs(row: Row) {
  const data = labels(row);
  const service = data["com.docker.compose.service"];
  const projectName = project(row);
  if (
    !projectName ||
    !service ||
    !/^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$/.test(service) ||
    data["com.docker.compose.oneoff"]?.toLowerCase() === "true"
  )
    return;
  const inputs = composeInputs(projectName, [row]);
  return inputs ? { ...inputs, services: [service] } : undefined;
}

export function containerActionUnavailable(
  action: "start" | "stop" | "restart" | "pause" | "unpause",
  row: Row,
): string | undefined {
  const state = String(row.State ?? "").toLowerCase();
  const allowed = {
    start: ["created", "exited"],
    stop: ["running", "paused", "restarting"],
    restart: ["running", "paused"],
    pause: ["running"],
    unpause: ["paused"],
  };
  if (allowed[action].includes(state)) return;
  if (action === "start" && state === "paused")
    return "Resume this paused container.";
  return `This action is unavailable while the container is ${state || "in an unknown state"}. Refresh or inspect its state first.`;
}

export function containerRemovalUnavailable(row: Row): string | undefined {
  const state = String(row.State ?? "").toLowerCase();
  if (["created", "exited", "dead"].includes(state)) return;
  return ["running", "paused", "restarting"].includes(state)
    ? "Stop the container before removing it."
    : "Refresh and inspect the container's state before removing it.";
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

// Native snapshots join Moby's bindings with the owned forwarder's actual
// listeners. A Docker running state alone cannot authorize an Open link.
export function publishedPorts(row: Row): Port[] {
  const verified = Array.isArray(row.OrbWinHostPorts)
    ? row.OrbWinHostPorts
    : [];
  return ports(row.Ports).map((port) => ({
    ...port,
    url: verified.some(
      (entry) =>
        entry &&
        typeof entry === "object" &&
        entry.port === port.hostPort &&
        entry.protocol === port.protocol &&
        entry.published === true,
    )
      ? port.url
      : undefined,
  }));
}
export function publicationWarning(row: Row): string {
  if (!isRunning(row)) return "";
  const bindings = ports(row.Ports);
  const verified = Array.isArray(row.OrbWinHostPorts)
    ? row.OrbWinHostPorts
    : [];
  const missing = bindings.filter(
    (port) =>
      !verified.some(
        (entry) =>
          entry &&
          typeof entry === "object" &&
          entry.port === port.hostPort &&
          entry.protocol === port.protocol &&
          entry.published === true,
      ),
  );
  if (!missing.length) return "";
  return typeof row.OrbWinPortError === "string"
    ? `Host publication is unverified: ${row.OrbWinPortError}`
    : `Windows publication is unavailable for ${missing.map((port) => `${port.hostPort}/${port.protocol}`).join(", ")}. The container may be running. Refresh, inspect a conflicting service or choose another port; OrbWin will not stop another engine.`;
}

export type SortMode = "status" | "name" | "size-desc" | "size-asc";
// Docker's rounded decimal image size is not reclaimable disk space.
export function imageSize(value: unknown): number | undefined {
  if (typeof value === "number")
    return Number.isSafeInteger(value) && value >= 0 ? value : undefined;
  if (typeof value !== "string") return;
  const match = value.trim().match(/^(\d+(?:\.\d+)?)\s*(B|kB|MB|GB|TB)$/);
  if (!match) return;
  const result =
    Number(match[1]) * 1000 ** ["B", "kB", "MB", "GB", "TB"].indexOf(match[2]);
  return Number.isFinite(result) && result <= Number.MAX_SAFE_INTEGER
    ? result
    : undefined;
}
export function platformBadge(os: unknown, architecture: unknown): string {
  if (
    typeof os !== "string" ||
    typeof architecture !== "string" ||
    !os ||
    !architecture
  )
    return "Platform unverified";
  return os === "linux" && architecture === "amd64"
    ? "Native linux/amd64"
    : `Unsupported ${os}/${architecture}`;
}
export function parseImagePlatform(output: string, id: string) {
  if (output.length > 1024 * 1024)
    throw new Error("Image metadata exceeded its display limit.");
  const values = JSON.parse(output);
  const image = Array.isArray(values) && values.length === 1 ? values[0] : null;
  const field = (value: unknown) =>
    typeof value === "string" && /^[a-z0-9][a-z0-9_.-]{0,63}$/.test(value);
  if (
    !image ||
    image.Id !== id ||
    !field(image.Os) ||
    !field(image.Architecture)
  )
    throw new Error(
      "Image metadata did not match the selected image or has an invalid platform.",
    );
  return { os: image.Os as string, architecture: image.Architecture as string };
}
export function parseContainerLimits(output: string, id: string) {
  if (output.length > 1024 * 1024)
    throw new Error("Container metadata exceeded its display limit.");
  const values = JSON.parse(output);
  const container =
    Array.isArray(values) && values.length === 1 ? values[0] : null;
  if (
    !container ||
    container.Id !== id ||
    !container.HostConfig ||
    typeof container.HostConfig !== "object" ||
    Array.isArray(container.HostConfig)
  )
    throw new Error("Container metadata did not match the selected container.");
  const number = (key: string) => {
    const value = container.HostConfig[key];
    if (value === undefined) return undefined;
    if (
      !Number.isSafeInteger(value) ||
      value < (key === "MemorySwap" || key === "CpuQuota" ? -1 : 0)
    )
      throw new Error("Container resource counters are invalid.");
    return value as number;
  };
  return {
    nanoCpus: number("NanoCpus"),
    cpuQuota: number("CpuQuota"),
    cpuPeriod: number("CpuPeriod"),
    memory: number("Memory"),
    memorySwap: number("MemorySwap"),
  };
}
export function compareRows(
  page: Page,
  mode: SortMode,
  sizes: ReadonlyMap<string, number> = new Map(),
) {
  return (a: Row, b: Row) => {
    if (
      (mode === "size-desc" || mode === "size-asc") &&
      (page === "images" || page === "volumes")
    ) {
      const size = (row: Row) =>
        page === "images" ? imageSize(row.Size) : sizes.get(String(row.Name));
      const left = size(a),
        right = size(b);
      if (left === undefined && right !== undefined) return 1;
      if (right === undefined && left !== undefined) return -1;
      if (left !== undefined && right !== undefined && left !== right)
        return mode === "size-desc" ? right - left : left - right;
    }
    if (mode === "status" && page === "containers") {
      const rank = (row: Row) => (isRunning(row) ? 2 : isPaused(row) ? 1 : 0);
      const order = rank(b) - rank(a);
      if (order) return order;
    }
    return name(a).localeCompare(name(b), undefined, { numeric: true });
  };
}

export type Entry =
  | { kind: "row"; row: Row }
  | { kind: "group"; project: string; rows: Row[] };
// Compose groups come only from returned project labels. Groups sort among
// standalone rows by the same rule: running if any member runs, otherwise paused
// if any member is paused. Paused never implies the container can execute work.
export function entries(
  page: Page,
  rows: Row[],
  mode: SortMode,
  sizes?: ReadonlyMap<string, number>,
): Entry[] {
  const compare = compareRows(page, mode, sizes);
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
          State: entry.rows.some(isRunning)
            ? "running"
            : entry.rows.some(isPaused)
              ? "paused"
              : "exited",
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
