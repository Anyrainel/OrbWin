import { Channel, invoke, isTauri } from "@tauri-apps/api/core";
import { dockerEndpoint } from "./model/client-targeting";

export type Inventory = {
  containers: Resource[];
  images: Resource[];
  volumes: Resource[];
  networks: Resource[];
};
export type Resource = Record<string, unknown>;
export type Diagnostic = { name: string; output: string; success: boolean };
export type BackendStatus = {
  setupAvailable?: boolean;
  upgradeAvailable?: boolean;
  resources?: { cpus?: number; memoryBytes?: number };
  available: boolean;
  endpoint: string;
  message: string;
  version?: string;
  diagnostic?: string;
};
export const endpoint = dockerEndpoint;
export async function diskInfo(): Promise<{
  path: string;
  fileLengthBytes: number;
}> {
  if (!isTauri())
    throw new Error(
      "Virtual disk measurements require the Windows desktop application.",
    );
  return invoke("engine_disk_info");
}

export type ResourceKind = "container" | "image" | "volume" | "network";
export type ClusterResource =
  | "events"
  | "pods"
  | "services"
  | "deployments"
  | "persistentVolumeClaims";
export type Operation =
  | { kind: "setup" }
  | { kind: "prepareUpdate" }
  | { kind: "registryList" }
  | { kind: "registryLogin"; server: string; username: string; secret: string }
  | { kind: "registryForget"; server: string }
  | { kind: "diskUsage" }
  | { kind: "files"; target: string; path: string; action: "list" | "read" }
  | {
      kind: "buildHistory";
      action: "list" | "inspect" | "logs";
      reference: string;
    }
  | { kind: "clusterLogs"; namespace: string; name: string; follow: boolean }
  | {
      kind: "clusterForward";
      resource: ClusterResource;
      namespace: string;
      name: string;
      local: string;
      remote: string;
    }
  | { kind: "clusterExec"; namespace: string; name: string; command: string[] }
  | { kind: "untag"; reference: string }
  | {
      kind: "clusterWait";
      resource: ClusterResource;
      namespace: string;
      name: string;
    }
  | { kind: "engine"; running: boolean }
  | { kind: "readSettings" }
  | { kind: "readClusterVersions" }
  | { kind: "configureCluster"; enabled: boolean; version?: string }
  | {
      kind: "cluster";
      action: "list" | "inspect" | "logs";
      resource: ClusterResource;
      namespace: string;
      name: string;
    }
  | { kind: "clusterApply"; file: string }
  | {
      kind: "clusterDelete";
      resource: ClusterResource;
      namespace: string;
      name: string;
      uid: string;
    }
  | { kind: "inspect"; resource: ResourceKind; target: string }
  | {
      kind: "container";
      action: "start" | "stop" | "restart" | "pause" | "unpause";
      target: string;
    }
  | { kind: "remove"; resource: ResourceKind; target: string }
  | { kind: "logs"; target: string; follow: boolean }
  | { kind: "stats"; target: string }
  | { kind: "pull"; image: string }
  | { kind: "tag"; target: string; tag: string }
  | { kind: "createVolume" | "createNetwork"; name: string }
  | {
      kind: "run";
      image: string;
      name: string;
      command: string[];
      options?: {
        ports?: string[];
        environment?: string[];
        volumes?: string[];
      };
    }
  | { kind: "exec"; target: string; command: string[] }
  | { kind: "build"; directory: string; tag: string }
  | {
      kind: "compose";
      file: string;
      project: string;
      action: "up" | "down" | "start" | "stop" | "restart" | "pull" | "logs";
    }
  | {
      kind: "connectNetwork";
      network: string;
      container: string;
      disconnect: boolean;
    };
export type Outcome = {
  exitCode: number | null;
  interrupted: boolean;
  mutation: boolean;
  success: boolean;
  message: string;
};
type Output =
  | { kind: "data"; stream: "stdout" | "stderr"; bytes: number[] }
  | { kind: "truncated" | "ready" };
export async function run(
  id: string,
  request: Operation,
  onOutput: (text: string, stream?: "stdout" | "stderr") => void,
  signal?: AbortSignal,
): Promise<Outcome> {
  if (!isTauri())
    throw new Error("Operations require the Windows desktop application.");
  const decoders = { stdout: new TextDecoder(), stderr: new TextDecoder() };
  const output = new Channel<Output>();
  let ready = false;
  const cancel = () => {
    if (ready)
      void cancelOperation(id).catch((error) =>
        onOutput(`\nCancellation failed: ${String(error)}\n`),
      );
  };
  signal?.addEventListener("abort", cancel, { once: true });
  output.onmessage = (event) => {
    if (event.kind === "ready") {
      ready = true;
      if (signal?.aborted) cancel();
    } else if (event.kind === "truncated")
      onOutput(
        "\n[Output limit reached; further output is discarded while the command finishes.]\n",
      );
    else if (event.kind === "data")
      onOutput(
        decoders[event.stream].decode(new Uint8Array(event.bytes), {
          stream: true,
        }),
        event.stream,
      );
  };
  try {
    return await invoke<Outcome>("engine_run", { id, request, output });
  } finally {
    signal?.removeEventListener("abort", cancel);
    for (const [stream, decoder] of Object.entries(decoders)) {
      const tail = decoder.decode();
      if (tail) onOutput(tail, stream as "stdout" | "stderr");
    }
  }
}
export function cancelOperation(id: string): Promise<void> {
  return invoke("engine_cancel", { id });
}
export function choosePath(directory: boolean): Promise<string | null> {
  return invoke("choose_path", { directory });
}
export function exportText(contents: string): Promise<boolean> {
  return invoke("export_text", { contents });
}
export function exportSupport(): Promise<boolean> {
  return invoke("export_support");
}

export async function status(): Promise<BackendStatus> {
  if (!isTauri())
    return {
      available: false,
      endpoint,
      message:
        "Open the Windows desktop application to check the local backend. This browser preview has no engine connection.",
    };
  return invoke<BackendStatus>("backend_status");
}
export async function inventory(): Promise<Inventory> {
  if (!isTauri())
    throw new Error("Inventory requires the Windows desktop application.");
  return invoke<Inventory>("engine_inventory");
}
export async function diagnostics(): Promise<Diagnostic[]> {
  if (!isTauri())
    throw new Error(
      "Host diagnostics require the Windows desktop application.",
    );
  return invoke<Diagnostic[]>("host_diagnostics");
}
