import {
  composeSpec,
  wslComposeSpec,
  containerActionSpec,
  removalDescriptions,
  removalRequest,
  removalSpec,
  runContainerSpec,
  type ContainerAction,
  type OperationSpec,
} from "../Operations";
import {
  composeInputs,
  composeServiceInputs,
  wslComposeInputs,
  labels,
  containerActionUnavailable,
  containerRemovalUnavailable,
  isRunning,
  name,
  publishedPorts,
  type Page,
  type Row,
} from "./model";

export type Action = {
  id: string;
  label: string;
  destructive?: boolean;
  spec?: OperationSpec;
  port?: number;
  copy?: string;
  // Present when the action cannot run; shown instead of silently hiding it.
  unavailable?: string;
};

const verbs: Record<ContainerAction, string> = {
  start: "Start",
  stop: "Stop",
  restart: "Restart",
  pause: "Pause",
  unpause: "Resume",
};
// One row reuses the existing single-resource dialog; several rows become one
// confirmed, sequential batch listing every affected resource.
function containerSpec(action: ContainerAction, rows: Row[]): OperationSpec {
  if (rows.length === 1) return containerActionSpec(action, rows[0]);
  return {
    title: `${verbs[action]} ${rows.length} containers?`,
    request: containerActionSpec(action, rows[0]).request,
    batch: rows.map((row) => ({
      label: name(row),
      request: containerActionSpec(action, row).request,
    })),
    description:
      action === "stop" || action === "restart"
        ? "Interrupts each listed container's workload, one at a time. Volumes are retained. Containers already processed are not rolled back if a later one fails or you cancel."
        : "Applies to each listed container on the OrbWin engine, one at a time. Completed items are not rolled back on failure or cancellation.",
  };
}
function removeSpec(page: Page, rows: Row[], note = ""): OperationSpec {
  if (rows.length === 1) {
    const spec = removalSpec(page, rows[0]);
    return note
      ? { ...spec, description: `${spec.description} ${note}` }
      : spec;
  }
  return {
    title: `Remove ${rows.length} ${page}?`,
    request: removalRequest(page, rows[0]),
    batch: rows.map((row) => ({
      label: name(row),
      request: removalRequest(page, row),
    })),
    destructive: true,
    description: `${removalDescriptions[page]} Each item is removed separately; rejected items are reported and do not stop the rest.${note ? ` ${note}` : ""}`,
  };
}

export function actionsFor(page: Page, rows: Row[]): Action[] {
  if (!rows.length) return [];
  const actions: Action[] = [];
  if (page === "volumes" && rows.length === 1) {
    const volume = String(rows[0].Name ?? "");
    actions.push({
      id: "clone-volume",
      label: "Clone volume…",
      spec: {
        title: `Clone ${volume}`,
        request: { kind: "cloneVolume", volume, name: "" },
        description:
          "Creates a full verified copy in a distinct new local volume. Stop writable users during snapshot capture; no workload is stopped automatically. Uses the same 64-GiB/100,000-entry portable metadata rules as export/restore. Allow a full guest copy plus metadata and roughly twice the payload size in temporary host staging. This is not copy-on-write. Interrupted copies retain a snapshot and/or isolated pending destination for explicit recovery.",
        fields: [{ key: "name", label: "New volume name" }],
      },
    });
    actions.push({
      id: "export-volume",
      label: "Export portable data…",
      spec: {
        title: `Export ${volume}`,
        request: { kind: "exportVolume", volume, destination: "" },
        description:
          "Stop writable users before exporting. Reads this ordinary local volume without changing it; external clients must keep it quiesced. Creates a new checksummed portable package, never overwriting a file. Supports up to 64 GiB and 100,000 entries, numeric ownership, modes, modification times and contained relative links. Extended attributes/ACLs, devices and escaping links refuse. Data may contain secrets. Host staging needs roughly twice the data size; interruption can retain named partial files.",
        fields: [
          { key: "destination", label: "New Windows package file", save: true },
        ],
      },
    });
  }
  if (page === "containers") {
    const stopped = rows.filter((row) => !containerRemovalUnavailable(row));
    const activeCount = rows.length - stopped.length;
    for (const action of [
      "start",
      "stop",
      "restart",
      "pause",
      "unpause",
    ] as const) {
      const eligible = rows.filter(
        (row) => !containerActionUnavailable(action, row),
      );
      if (eligible.length)
        actions.push({
          id: action,
          label: verbs[action],
          spec: containerSpec(action, eligible),
        });
    }
    if (rows.length === 1)
      for (const port of publishedPorts(rows[0]))
        if (port.url && isRunning(rows[0]))
          actions.push(
            {
              id: `open-${port.hostPort}`,
              label: `Open localhost:${port.hostPort}`,
              port: port.hostPort,
            },
            {
              id: `copy-${port.hostPort}`,
              label: `Copy http://localhost:${port.hostPort}`,
              copy: port.url,
            },
          );
    actions.push(
      stopped.length
        ? {
            id: "remove",
            label: "Remove",
            destructive: true,
            spec: removeSpec(
              page,
              stopped,
              activeCount
                ? `${activeCount} active or unverified container${activeCount === 1 ? " is" : "s are"} not included; stop or inspect ${activeCount === 1 ? "it" : "them"} first.`
                : "",
            ),
          }
        : {
            id: "remove",
            label: "Remove",
            destructive: true,
            unavailable:
              "Stop active containers and verify their state before removing them.",
          },
    );
    if (rows.length === 1) {
      const inputs = composeServiceInputs(rows[0]);
      if (inputs)
        for (const action of ["logs", "up", "pull"] as const)
          actions.push({
            id: `compose-service-${action}`,
            label: `Compose service ${action}…`,
            spec: composeSpec(
              action,
              inputs,
              "Selects the exact service label reported by this container, including its replicas. Up may also start dependencies and may recreate containers; this does not target only the selected container.",
            ),
          });
      const wsl = wslComposeInputs(
        labels(rows[0])["com.docker.compose.project"],
        [rows[0]],
      );
      const service = labels(rows[0])["com.docker.compose.service"];
      if (
        wsl &&
        wsl.signature &&
        service &&
        /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$/.test(service) &&
        labels(rows[0])["com.docker.compose.oneoff"]?.toLowerCase() !== "true"
      ) {
        for (const action of ["logs", "up", "pull"] as const)
          actions.push({
            id: `wsl-compose-service-${action}`,
            label: `WSL service ${action}…`,
            spec: wslComposeSpec(wsl.distribution, wsl.source, action, {
              ...wsl,
              services: [service],
            }),
          });
      }
      if (wsl && !wsl.signature)
        actions.push({
          id: "wsl-review-connection",
          label: "WSL service actions…",
          unavailable:
            "These older project labels have no source identity. Inspect the intended project in WSL distributions before running its Compose workflow.",
        });
    }
    return actions;
  }
  if (page === "images" && rows.length === 1) {
    const row = rows[0];
    const reference =
      row.Repository && row.Repository !== "<none>" && row.Tag !== "<none>"
        ? `${row.Repository}:${row.Tag}`
        : String(row.ID);
    actions.push({
      id: "run",
      label: "Run…",
      spec: runContainerSpec(reference),
    });
    actions.push({
      id: "export-image",
      label: "Export image archive…",
      spec: {
        title: `Export ${reference}`,
        request: {
          kind: "exportImage",
          image: String(row.ID),
          reference,
          destination: "",
        },
        description:
          "Saves the reviewed image to a new uncompressed Docker .tar archive, up to 64 GiB. Verifies config and layer identities before publication. Existing files are never replaced. Allow space for the full archive plus headroom. Source images/tags are unchanged. Untagged exports retain content IDs; architectures in an archive do not imply runtime emulation support. Cancellation can leave a named partial file for inspection.",
        fields: [
          { key: "destination", label: "New .tar archive path", save: true },
        ],
      },
    });
  }
  actions.push({
    id: "remove",
    label: "Remove",
    destructive: true,
    spec: removeSpec(page, rows),
  });
  return actions;
}

// Discovery only prefills an editable review; inconsistent metadata never chooses a winner.
export function groupActions(project: string, rows: Row[]): Action[] {
  const wsl = wslComposeInputs(project, rows);
  if (wsl && !wsl.signature)
    return [
      ...actionsFor("containers", rows).filter(
        (action) => !action.port && !action.copy,
      ),
      {
        id: "wsl-review-connection",
        label: "WSL Compose actions…",
        unavailable:
          "These older project labels have no source identity. Inspect the intended project in WSL distributions before running its Compose workflow.",
      },
    ];
  if (wsl)
    return [
      ...actionsFor("containers", rows).filter(
        (action) => !action.port && !action.copy,
      ),
      ...(["validate", "up", "pull", "logs", "down"] as const).map(
        (action) => ({
          id: `wsl-compose-${action}`,
          label: `WSL Compose ${action}…`,
          destructive: action === "down",
          spec: wslComposeSpec(wsl.distribution, wsl.source, action, wsl),
        }),
      ),
    ];
  const inputs = composeInputs(project, rows);
  const compose = (
    action: "validate" | "up" | "pull" | "logs" | "down",
    label: string,
  ): Action => ({
    id: `compose-${action}`,
    label,
    destructive: action === "down",
    spec: composeSpec(
      action,
      inputs ?? {
        project,
        file: "",
        overrides: [],
        profiles: [],
        directory: "",
      },
      inputs
        ? "Prefilled from consistent engine labels. Profiles are not inferred from running services."
        : "Project labels are missing, conflicting, ambiguous or use unsupported paths/environment files. Supply the intended project inputs explicitly; per-container actions remain available.",
    ),
  });
  return [
    ...actionsFor("containers", rows).filter(
      (action) => !action.port && !action.copy,
    ),
    compose("validate", "Validate project…"),
    compose("up", "Compose up…"),
    compose("pull", "Pull project images…"),
    compose("logs", "Project logs…"),
    compose("down", "Compose down…"),
  ];
}
