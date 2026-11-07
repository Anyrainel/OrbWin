import { Channel, invoke, isTauri } from "@tauri-apps/api/core";
import { dockerEndpoint } from "./model/client-targeting";
export function openSetupLogs(): Promise<void> {
  if (!isTauri()) return Promise.reject({ code: "unavailable" });
  return invoke("open_setup_logs");
}
export type ClientRecoveryReview = {
  epoch: string;
  summary: string;
  expiresInSeconds: number;
};
export async function reviewClientRecovery(): Promise<ClientRecoveryReview | null> {
  if (!isTauri())
    throw new Error("Recovery requires the Windows desktop application.");
  return invoke<ClientRecoveryReview | null>("client_recovery_review");
}
export async function cancelClientRecovery(epoch: string): Promise<void> {
  return invoke("client_recovery_cancel", { epoch });
}
export async function restoreClientRecovery(epoch: string): Promise<void> {
  return invoke("client_recovery_restore", { epoch });
}
import type { EngineProxyReview } from "./model/engine-proxy";
import type {
  LANAccessStatus,
  LANOptions,
  LANReview,
  LANReconciliation,
  LANSelection,
} from "./model/lan-access";

// Explicit private client setup. No project, distribution, or caller path is sent.
export async function refreshWslClients(): Promise<{ changed: boolean }> {
  if (!isTauri())
    throw new Error(
      "Client tool refresh requires the Windows desktop application.",
    );
  return invoke<{ changed: boolean }>("wsl_refresh_private_clients");
}
export async function engineProxyCapability(): Promise<boolean> {
  if (!isTauri()) return false;
  return invoke<boolean>("engine_proxy_capability");
}

export type Inventory = {
  containers: Resource[];
  images: Resource[];
  volumes: Resource[];
  networks: Resource[];
};
export type Resource = Record<string, unknown>;
export type ImageImportChoice = {
  reference: string;
  configId: string;
  expectedId: string;
  apply: boolean;
  replace: boolean;
};

export type LocalServiceSelector = {
  containerId?: string;
  project?: string;
  service?: string;
  replica?: number;
};
export type LocalServicePreference = {
  selector: LocalServiceSelector;
  disabled?: boolean;
  name?: string;
  port?: number;
  routes?: { name: string; port: number; https?: boolean }[];
};
export type LocalServiceSettings = {
  enabled: boolean;
  ports: { http: number; https: number };
  rootSha256?: string;
  preferences?: LocalServicePreference[];
  networks?: string[];
};
export type WslTrustReview = {
  distributionId: string;
  machineId?: string;
  guestSequence?: number;
  ownerSequence?: number;
};
export type WslTrustStatus = {
  distributionId: string;
  rootSha256: string;
  ownerState: string;
  ownerSequence: number;
  observed: boolean;
  managed: boolean;
  guest?: {
    distributionId: string;
    machineId: string;
    osId: string;
    osVersion: string;
    rootSha256: string;
    observed: boolean;
    filePresent: boolean;
    bundlePresent: boolean;
    managed: boolean;
    state: string;
    sequence: number;
    updaterWarning: boolean;
  };
};
export type WslTrustReceipt = {
  distributionId: string;
  machineId: string;
  rootSha256: string;
  state: string;
  ownerSequence: number;
  guestSequence: number;
  registrationState: "present" | "absent" | "unknown";
  registrationError?: string;
  retiredAt?: string;
};
export type LocalServiceState = {
  instance: string;
  sequence: number;
  revision: number;
  settings: LocalServiceSettings;
  phase: string;
  updatedAt: string;
  inventoryReadAt?: string;
  lan?: LANAccessStatus[];
  wslDistributionsRead?: boolean;
  wslDistributions?: { id: string; name: string }[];
  wslTrust?: WslTrustStatus;
  wslTrustReceipts?: WslTrustReceipt[];
  wslTrustReceiptsRead?: boolean;
  wslTrustReceiptsError?: string;
  windowsTrust?: {
    rootSha256: string;
    present: boolean;
    observed: boolean;
    managed: boolean;
    state: string;
    sequence: number;
  };
  rotation?: {
    oldRootSha256: string;
    newRootSha256: string;
    ports: { http: number; https: number };
    oldRevision: number;
    targetRevision: number;
    oldCertificatePem: string;
  };
  error?: string;
  candidates?: {
    containerId: string;
    containerName: string;
    selector: LocalServiceSelector;
    names: string[];
    ports: number[];
    selectedPort?: number;
    routes?: { name: string; port: number; https?: boolean }[];
    state: string;
    reason?: string;
  }[];
  services?: {
    containerId: string;
    names: string[];
    port?: number;
    state: string;
    reason?: string;
  }[];
  authority?: {
    rootSha256: string;
    rootNotBefore: string;
    rootNotAfter: string;
    publicCertificatePem: string;
  };
};
export async function localServices(
  request:
    | {
        operation:
          | "snapshot"
          | "preview"
          | "retry"
          | "wsl-trust-distributions"
          | "wsl-trust-receipts"
          | "lan-options";
      }
    | {
        operation:
          | "wsl-trust-status"
          | "wsl-trust-install"
          | "wsl-trust-remove"
          | "retire-wsl-trust-receipt";
        revision: number;
        rootSha256: string;
        wslReview: WslTrustReview;
      }
    | {
        operation: "configure";
        revision: number;
        settings: LocalServiceSettings;
      }
    | { operation: "create-authority"; revision: number }
    | { operation: "adopt-authority"; revision: number; rootSha256: string }
    | {
        operation: "trust-status" | "trust-install" | "trust-remove";
        revision: number;
        rootSha256: string;
      }
    | {
        operation: "rotate-authority";
        revision: number;
        rootSha256: string;
        ports: { http: number; https: number };
      }
    | {
        operation: "resume-authority-rotation" | "cancel-authority-rotation";
        revision: number;
        rootSha256: string;
        newRootSha256: string;
      }
    | { operation: "lan-review"; revision: number; lanSelection: LANSelection }
    | {
        operation:
          | "lan-activate"
          | "lan-remove"
          | "lan-settle-remove"
          | "lan-cleanup";
        revision: number;
        lanReviewId: string;
        lanReviewSha256: string;
      }
    | { operation: "lan-cancel" | "lan-settle-helper"; lanOperationId: string }
    | {
        operation: "lan-reconcile-review";
        lanReviewId: string;
        lanReviewSha256: string;
      }
    | {
        operation: "lan-cleanup-review";
        lanReviewId: string;
        lanReviewSha256: string;
      }
    | {
        operation:
          | "lan-remove-review"
          | "lan-settle-remove-review"
          | "lan-inspect";
        lanLeaseSha256: string;
      },
): Promise<{
  state?: LocalServiceState;
  lanOptions?: LANOptions;
  lanReview?: LANReview;
  lanReconciliation?: LANReconciliation;
  error?: string;
}> {
  if (!isTauri())
    throw new Error(
      "Local service setup requires the Windows desktop application.",
    );
  return invoke("local_services_request", { request });
}
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
  hostCapacity?: {
    directory: string;
    availableBytes: number;
    totalBytes: number;
    freeBytes: number;
  };
  hostCapacityError?: string;
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
  | {
      kind: "wslSaveMapping";
      distribution: string;
      connection: { registration: string; source: string; signature: string };
      file: string;
      contents: string;
      target: string;
    }
  | { kind: "volumeSize"; target: string; createdAt: string }
  | { kind: "recoverTools" }
  | {
      kind: "wslCompose";
      reviewedDaemon?: string;
      mappingReview?: { file: string; sha256: string; contents: string };
      distribution: string;
      connection?: { registration?: string; source: string; signature: string };
      file: string;
      overrides?: string[];
      envFiles?: string[];
      project: string;
      directory?: string;
      profiles?: string[];
      services?: string[];
      action:
        | "validate"
        | "up"
        | "down"
        | "start"
        | "stop"
        | "restart"
        | "pull"
        | "logs";
    }
  | {
      kind: "wslBuild";
      distribution: string;
      connection?: { registration?: string; source: string; signature: string };
      directory: string;
      tag: string;
    }
  | { kind: "volumeTransfers" }
  | { kind: "discardVolumeTransfer"; transaction: string }
  | { kind: "exportVolume"; volume: string; destination: string }
  | {
      kind: "exportImage";
      image: string;
      reference: string;
      destination: string;
    }
  | { kind: "restoreVolume"; source: string; name: string }
  | { kind: "reviewImageImport"; source: string }
  | { kind: "reviewDesktopMigration" }
  | { kind: "migrationWslSources" }
  | { kind: "reviewRancherMigration"; registration: string; machine: string }
  | {
      kind: "exportRancherMigrationImage";
      source: string;
      registration: string;
      machine: string;
      image: string;
      reference: string;
      destination: string;
    }
  | { kind: "reviewDesktopMigrationVolume"; source: string; volume: string }
  | {
      kind: "reviewRancherMigrationVolume";
      source: string;
      registration: string;
      machine: string;
      volume: string;
    }
  | {
      kind: "exportRancherMigrationVolume";
      source: string;
      registration: string;
      machine: string;
      volume: string;
      fingerprint: string;
      destination: string;
    }
  | {
      kind: "exportDesktopMigrationVolume";
      source: string;
      volume: string;
      fingerprint: string;
      destination: string;
    }
  | { kind: "recoverDesktopMigrationVolume"; transaction: string }
  | {
      kind: "exportDesktopMigrationImage";
      source: string;
      image: string;
      reference: string;
      destination: string;
    }
  | {
      kind: "importImages";
      source: string;
      sha256: string;
      tags: ImageImportChoice[];
    }
  | { kind: "imageImportHistory" }
  | { kind: "migrationVolumeHistory" }
  | { kind: "migrationImageHistory" }
  | {
      kind: "reviewMigrationDestination";
      source: string;
      sourceVolume: string;
      destination: string;
    }
  | { kind: "imageImportRecord"; transaction: string }
  | { kind: "cloneVolume"; volume: string; name: string }
  | { kind: "volumeClones" }
  | { kind: "discardCloneStaging"; transaction: string }
  | {
      kind: "saveVolumeFile";
      volume: string;
      path: string;
      expected: string;
      text: string;
    }
  | {
      kind: "copyFile";
      resource: "container" | "image" | "volume";
      target: string;
      path: string;
      destination: string;
    }
  | {
      kind: "files";
      resource?: "container" | "image" | "volume";
      target: string;
      path: string;
      action: "list" | "read";
    }
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
  | { kind: "configureEngineProxy"; review: EngineProxyReview }
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
  | { kind: "engineStats" }
  | { kind: "pull"; image: string }
  | { kind: "tag"; target: string; tag: string }
  | { kind: "createVolume"; name: string }
  | {
      kind: "createNetwork";
      name: string;
      driver?: string;
      subnet?: string;
      internal?: boolean;
    }
  | {
      kind: "run";
      image: string;
      name: string;
      command: string[];
      options?: {
        ports?: string[];
        environment?: string[];
        volumes?: string[];
        restart?: string;
        network?: string;
        cpus?: string;
        memoryMiB?: string;
      };
    }
  | {
      kind: "updateContainerLimits";
      target: string;
      cpus: string;
      memoryMiB: string;
    }
  | { kind: "exec"; target: string; command: string[] }
  | { kind: "build"; directory: string; tag: string }
  | {
      kind: "compose";
      file: string;
      overrides?: string[];
      envFiles?: string[];
      profiles?: string[];
      services?: string[];
      directory?: string;
      project: string;
      action:
        | "validate"
        | "up"
        | "down"
        | "start"
        | "stop"
        | "restart"
        | "pull"
        | "logs";
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
  | { kind: "setupDecision"; reason: "prior-app-absent"; nonce: string }
  | { kind: "setupPaused" }
  | { kind: "truncated" | "ready" };
export type SetupDecision = { reason: "prior-app-absent"; nonce: string };
export async function run(
  id: string,
  request: Operation,
  onOutput: (text: string, stream?: "stdout" | "stderr") => void,
  signal?: AbortSignal,
  onSetupDecision?: (decision: SetupDecision) => void,
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
    } else if (event.kind === "setupDecision") {
      if (request.kind === "setup" && onSetupDecision)
        onSetupDecision({ reason: event.reason, nonce: event.nonce });
      else cancel();
    } else if (event.kind === "setupPaused") {
      onOutput("\nSetup is paused. Existing files and logs were kept.\n");
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
export function continueSetup(id: string, nonce: string): Promise<void> {
  return invoke("engine_setup_decide", { id, nonce });
}
export function choosePath(directory: boolean): Promise<string | null> {
  return invoke("choose_path", { directory });
}
export function chooseSavePath(): Promise<string | null> {
  return invoke("choose_save_path");
}
export function openSelectedCopy(edit: boolean): Promise<boolean> {
  return invoke("open_selected_copy", { edit });
}
export function openLocalCopy(path: string, edit: boolean): Promise<void> {
  return invoke("open_local_copy", { path, edit });
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

export type NetworkProbe = {
  state:
    | "not-run"
    | "verified-owned-listener"
    | "failed"
    | "listen-unavailable";
  errorClass?:
    | "canceled"
    | "timeout"
    | "not-found"
    | "unavailable"
    | "ownership-mismatch";
};
export type NetworkProxyConfiguration = {
  source: string;
  state: "observed" | "unavailable";
  mode:
    | "unknown"
    | "direct-configured"
    | "automatic-configured"
    | "manual-configured"
    | "mixed-configured";
  autoDetect: boolean;
  pacConfigured: boolean;
  manualConfigured: boolean;
  bypassConfigured: boolean;
  effectiveProxy: "not-evaluated";
};
export type NetworkDiagnosticsSnapshot = {
  schema: 1;
  loopback: {
    version: 1;
    readAt: string;
    source: string;
    ipv4: NetworkProbe;
    ipv6: NetworkProbe;
    names: {
      name: "localhost" | "network-check.orbwin.localhost";
      resolution:
        | "observed"
        | "canceled"
        | "timeout"
        | "not-found"
        | "unavailable"
        | "invalid-answer-count"
        | "malformed-answer"
        | "non-loopback-answer";
      families: ("ipv4" | "ipv6")[];
      nonLoopbackAnswers: number;
      firstFamily?: "ipv4" | "ipv6" | null;
      nativeDial: NetworkProbe;
      ipv4OnlyDial: NetworkProbe;
    }[];
  };
  proxy: {
    version: 1;
    currentUser: NetworkProxyConfiguration;
    winHttpDefault: NetworkProxyConfiguration;
  };
  complete: boolean;
};
export async function networkDiagnostics(): Promise<NetworkDiagnosticsSnapshot> {
  if (!isTauri())
    throw new Error("Network checks require the Windows desktop application.");
  return invoke<NetworkDiagnosticsSnapshot>("network_diagnostics");
}

export type DesktopPreferences = {
  enabled: boolean;
  trayAvailable: boolean;
  trayIssue: string | null;
};
export function desktopPreferences(): Promise<DesktopPreferences> {
  if (!isTauri())
    return Promise.reject(
      new Error("Sign-in preferences require the Windows desktop application."),
    );
  return invoke<DesktopPreferences>("desktop_preferences");
}
export function desktopSetStartup(
  enabled: boolean,
): Promise<DesktopPreferences> {
  if (!isTauri())
    return Promise.reject(
      new Error("Sign-in preferences require the Windows desktop application."),
    );
  return invoke<DesktopPreferences>("desktop_set_startup", { enabled });
}
