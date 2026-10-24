import {
  containerActionSpec,
  removalDescriptions,
  removalRequest,
  removalSpec,
  runContainerSpec,
  type ContainerAction,
  type OperationSpec,
} from "../Operations";
import {
  composeFile,
  isRunning,
  name,
  ports,
  type Page,
  type Row,
} from "./model";

export type Action = {
  id: string;
  label: string;
  destructive?: boolean;
  spec?: OperationSpec;
  href?: string;
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
  if (page === "containers") {
    const running = rows.filter(isRunning);
    const stopped = rows.filter((row) => !isRunning(row));
    if (stopped.length)
      actions.push({
        id: "start",
        label: "Start",
        spec: containerSpec("start", stopped),
      });
    if (running.length)
      actions.push(
        { id: "stop", label: "Stop", spec: containerSpec("stop", running) },
        {
          id: "restart",
          label: "Restart",
          spec: containerSpec("restart", running),
        },
      );
    if (rows.length === 1)
      for (const port of ports(rows[0].Ports))
        if (port.url && isRunning(rows[0]))
          actions.push(
            {
              id: `open-${port.hostPort}`,
              label: `Open localhost:${port.hostPort}`,
              href: port.url,
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
              running.length
                ? `${running.length} running container${running.length === 1 ? " is" : "s are"} not included; stop ${running.length === 1 ? "it" : "them"} first.`
                : "",
            ),
          }
        : {
            id: "remove",
            label: "Remove",
            destructive: true,
            unavailable: "Stop running containers before removing them.",
          },
    );
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
  }
  actions.push({
    id: "remove",
    label: "Remove",
    destructive: true,
    spec: removeSpec(page, rows),
  });
  return actions;
}

// Compose-level actions only when the engine recorded one Windows Compose file;
// otherwise the group acts through its listed containers and says so.
export function groupActions(project: string, rows: Row[]): Action[] {
  const file = composeFile(rows);
  const compose = (
    action: "logs" | "down",
    label: string,
    destructive = false,
  ): Action =>
    file
      ? {
          id: `compose-${action}`,
          label,
          destructive,
          spec: {
            title: `Compose ${action} ${project}${destructive ? "?" : ""}`,
            request: { kind: "compose", file, project, action },
            destructive,
            description: `Uses the Compose file recorded by the engine: ${file}. If it was moved, edited or deleted, the command fails or applies the current file.${destructive ? " Stops and removes this project's containers and networks. Volumes are retained." : ""}`,
          },
        }
      : {
          id: `compose-${action}`,
          label,
          destructive,
          unavailable:
            "The engine did not record a single Windows Compose file for this project. Use per-container actions instead.",
        };
  return [
    ...actionsFor("containers", rows).filter(
      (action) => !action.href && !action.copy,
    ),
    compose("logs", "Project logs…"),
    compose("down", "Compose down…", true),
  ];
}
