import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import * as backend from "../backend";
import { LocalPortLink } from "./LocalPortLink";
import {
  containerActionUnavailable,
  containerRemovalUnavailable,
} from "./resources/model";
import type { ResourcePage } from "../model/navigation";
import { forwardUrls } from "./cluster/model";

type Field = {
  key: string;
  label: string;
  multiline?: boolean;
  placeholder?: string;
};
type Spec = {
  title: string;
  request: backend.Operation;
  description?: string;
  destructive?: boolean;
  fields?: Field[];
  // Initial field values; the user still reviews and submits them.
  defaults?: Record<string, string>;
  // Runs these requests sequentially after one confirmation instead of `request`.
  batch?: { label: string; request: backend.Operation }[];
};
const Context = createContext<(spec: Spec) => void>(() => {});
export type OperationSpec = Spec;
/** Opens the shared confirmation/output dialog for a prepared operation. */
export function useOperation() {
  return useContext(Context);
}
export function ClusterAccess({
  resource,
  metadata,
}: {
  resource: backend.ClusterResource;
  metadata: { name: string; namespace: string };
}) {
  const open = useContext(Context);
  return (
    <div className="action-row">
      {(resource === "pods" || resource === "services") && (
        <button
          className="button"
          onClick={() =>
            open({
              title: `Forward a port to ${metadata.name}`,
              request: {
                kind: "clusterForward",
                resource,
                ...metadata,
                local: "",
                remote: "",
              },
              fields: [
                {
                  key: "local",
                  label: "Local port (0 allocates an available localhost port)",
                },
                { key: "remote", label: "Remote port" },
              ],
              description:
                "Opens a localhost-only forward in the Port forwards panel. It stays connected while navigating. Stop it from that panel without stopping the workload.",
            })
          }
        >
          Forward port
        </button>
      )}
      {resource === "pods" && (
        <button
          className="button"
          onClick={() =>
            open({
              title: `Run command in ${metadata.name}`,
              request: { kind: "clusterExec", ...metadata, command: [] },
              fields: [
                {
                  key: "command",
                  label: "Executable, then one argument per line",
                  multiline: true,
                },
              ],
              description:
                "Executes in the pod's default container. Cancellation closes the client; a process may remain inside the pod.",
            })
          }
        >
          Run command
        </button>
      )}
      <button
        className="button"
        onClick={() =>
          open({
            title: `Events in ${metadata.namespace}`,
            request: {
              kind: "cluster",
              resource: "events",
              action: "list",
              namespace: metadata.namespace,
              name: "",
            },
          })
        }
      >
        Read events
      </button>
    </div>
  );
}
export function ClusterWait({
  resource,
  metadata,
}: {
  resource: backend.ClusterResource;
  metadata: { name: string; namespace: string };
}) {
  const open = useContext(Context);
  return resource === "services" || resource === "events" ? null : (
    <button
      className="button"
      onClick={() =>
        open({
          title: `Wait for ${metadata.name}`,
          request: {
            kind: "clusterWait",
            resource,
            name: metadata.name,
            namespace: metadata.namespace,
          },
          description:
            "Waits up to two minutes for the engine-reported ready, available or bound condition. Cancelling this read does not stop the workload.",
        })
      }
    >
      Wait for readiness
    </button>
  );
}
// `lifecycle={false}` hides start/stop where the shell's shared engine
// lifecycle control is shown instead, leaving the raw configuration read.
export function EngineControls({
  lifecycle = true,
}: { lifecycle?: boolean } = {}) {
  const open = useContext(Context);
  return (
    <div className="action-row">
      {lifecycle && (
        <button
          className="button"
          onClick={() =>
            open({
              title: "Start OrbWin engine",
              request: { kind: "engine", running: true },
              description:
                "Starts the recorded owned guest through rdd. Other engine state is unchanged. First-run provisioning remains a separate implementation step.",
            })
          }
        >
          Start engine
        </button>
      )}
      {lifecycle && (
        <button
          className="button"
          onClick={() =>
            open({
              title: "Stop OrbWin engine?",
              request: { kind: "engine", running: false },
              description:
                "Stops the OrbWin guest and interrupts its containers and cluster. Persistent disks are retained. Other engines and user distributions are not stopped.",
            })
          }
        >
          Stop engine
        </button>
      )}
      <button
        className="button"
        onClick={() =>
          open({
            title: "Engine configuration",
            request: { kind: "readSettings" },
          })
        }
      >
        Read configuration
      </button>
    </div>
  );
}
export function ClusterControls({
  only,
}: {
  only?: "enable" | "disable";
} = {}) {
  const open = useContext(Context);
  return (
    <div className="action-row">
      {[true, false]
        .filter((enabled) => !only || enabled === (only === "enable"))
        .map((enabled) => (
          <button
            key={String(enabled)}
            className="button"
            onClick={() =>
              open({
                title: `${enabled ? "Enable" : "Disable"} Kubernetes?`,
                request: { kind: "configureCluster", enabled },
                description: enabled
                  ? "Sets up the optional workload cluster in the owned OrbWin guest. This can download cluster components and take several minutes. Docker workflows remain independent of the cluster."
                  : "Stops the OrbWin workload cluster and interrupts its Kubernetes workloads. Other engines and global Kubernetes configuration are not changed.",
              })
            }
          >
            {enabled ? "Enable Kubernetes" : "Disable Kubernetes"}
          </button>
        ))}
    </div>
  );
}
export function ClusterManifest() {
  const open = useContext(Context);
  return (
    <button
      className="button"
      onClick={() =>
        open({
          title: "Apply Kubernetes manifest",
          request: { kind: "clusterApply", file: "" },
          fields: [{ key: "file", label: "Absolute manifest file path" }],
          description:
            "Creates or updates resources in the owned workload cluster using this manifest. Workloads can run privileged containers and mount guest data; select a file you trust. This does not apply anything to another Kubernetes context.",
        })
      }
    >
      Apply manifest
    </button>
  );
}
export function ClusterRemoval({
  resource,
  metadata,
  className = "button danger",
  label = "Delete resource",
  ariaLabel,
}: {
  resource: backend.ClusterResource;
  metadata: { name: string; namespace: string; uid: string };
  className?: string;
  label?: ReactNode;
  ariaLabel?: string;
}) {
  const open = useContext(Context);
  return (
    <button
      className={className}
      aria-label={ariaLabel}
      title={ariaLabel}
      onClick={() =>
        open({
          title: `Delete ${metadata.namespace}/${metadata.name}?`,
          request: { kind: "clusterDelete", resource, ...metadata },
          destructive: true,
          description:
            resource === "persistentVolumeClaims"
              ? "Deletes this storage claim. The storage class reclaim policy may permanently delete its data. Back up required data before proceeding."
              : "Deletes this resource from the owned workload cluster. Dependent workloads may be interrupted or removed. Controllers may recreate managed pods. A UID precondition prevents deleting a replacement object with the same name.",
        })
      }
    >
      {label}
    </button>
  );
}
export function OperationProvider({
  children,
  onMutation,
}: {
  children: ReactNode;
  onMutation: () => Promise<void>;
}) {
  const [spec, setSpec] = useState<Spec | null>(null);
  const forwards = useRef(new Map<string, AbortController>());
  const [forwardViews, setForwardViews] = useState<
    {
      id: string;
      label: string;
      output: string;
      active: boolean;
      stopping: boolean;
      message: string;
    }[]
  >([]);
  useEffect(
    () => () => {
      for (const abort of forwards.current.values()) abort.abort();
    },
    [],
  );
  const [values, setValues] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [output, setOutput] = useState("");
  const [result, setResult] = useState("");
  const dialog = useRef<HTMLDialogElement>(null);
  const active = useRef(false);
  const controller = useRef<AbortController | null>(null);
  const trigger = useRef<HTMLElement | null>(null);
  const open = (next: Spec) => {
    if (active.current) return;
    trigger.current = document.activeElement as HTMLElement;
    setSpec(next);
    setValues(next.defaults ?? {});
    setOutput("");
    setResult("");
  };
  useEffect(() => {
    if (spec) dialog.current?.showModal();
    else trigger.current?.focus();
  }, [spec]);
  function close() {
    if (active.current) {
      controller.current?.abort();
      return;
    }
    dialog.current?.close();
    setSpec(null);
  }
  async function submit() {
    if (!spec || active.current) return;
    let request = { ...spec.request };
    let mutation = true;
    try {
      for (const field of spec.fields ?? []) {
        const raw = values[field.key] ?? "";
        if (
          request.kind === "run" &&
          ["ports", "environment", "volumes"].includes(field.key)
        ) {
          request = {
            ...request,
            options: {
              ...request.options,
              [field.key]: raw.split(/\r?\n/).filter(Boolean),
            },
          };
          continue;
        }
        request = {
          ...request,
          [field.key]:
            field.key === "command"
              ? raw === ""
                ? []
                : raw.split(/\r?\n/)
              : raw.trim(),
        };
      }
      if (request.kind === "clusterForward") {
        if (forwards.current.size >= 2)
          throw new Error(
            "Two forwards are already running. Stop one before opening another.",
          );
        const id = crypto.randomUUID();
        const abort = new AbortController();
        forwards.current.set(id, abort);
        const label = `${request.namespace}/${request.name}:${request.remote}`;
        setForwardViews((previous) => [
          ...previous.filter((item) => item.active),
          ...previous.filter((item) => !item.active).slice(-6),
          {
            id,
            label,
            output: "",
            active: true,
            stopping: false,
            message: "Connecting…",
          },
        ]);
        const update = (change: Partial<(typeof forwardViews)[number]>) =>
          setForwardViews((previous) =>
            previous.map((item) =>
              item.id === id ? { ...item, ...change } : item,
            ),
          );
        void backend
          .run(
            id,
            request,
            (chunk) =>
              setForwardViews((previous) =>
                previous.map((item) =>
                  item.id === id
                    ? { ...item, output: (item.output + chunk).slice(-16384) }
                    : item,
                ),
              ),
            abort.signal,
          )
          .then((outcome) => update({ message: outcome.message }))
          .catch((error) => update({ message: String(error) }))
          .finally(() => {
            forwards.current.delete(id);
            update({ active: false });
          });
        mutation = false;
        dialog.current?.close();
        setSpec(null);
        return;
      }
      active.current = true;
      setPending(true);
      setResult("");
      setOutput("");
      const abort = new AbortController();
      controller.current = abort;
      const append = (chunk: string) =>
        setOutput((previous) => (previous + chunk).slice(-131072));
      if (spec.batch) {
        // Sequential and non-atomic: report each item; stop starting new ones
        // after cancellation. Earlier items may already have changed state.
        const failed: string[] = [];
        let done = 0;
        for (const item of spec.batch) {
          if (abort.signal.aborted) break;
          append(`
── ${item.label}
`);
          try {
            const outcome = await backend.run(
              crypto.randomUUID(),
              item.request,
              append,
              abort.signal,
            );
            append(`${outcome.message}
`);
            if (!outcome.success) failed.push(item.label);
          } catch (error) {
            append(`${String(error)}
`);
            failed.push(item.label);
          }
          done++;
        }
        setResult(
          `${done - failed.length} of ${spec.batch.length} succeeded.` +
            (failed.length ? ` Failed: ${failed.join(", ")}.` : "") +
            (done < spec.batch.length
              ? ` Cancelled; ${spec.batch.length - done} not started.`
              : ""),
        );
      } else {
        const outcome = await backend.run(
          crypto.randomUUID(),
          request,
          append,
          abort.signal,
        );
        mutation = outcome.mutation;
        setResult(outcome.message);
      }
    } catch (error) {
      setResult(String(error));
    } finally {
      // Even a failed/cancelled mutation may have committed. Reconcile via reads.
      try {
        if (mutation) await onMutation();
      } catch (error) {
        setResult(
          (previous) =>
            `${previous}\nState refresh failed: ${String(error)}. Refresh before another action.`,
        );
      }
      if (
        ["clusterApply", "clusterDelete", "configureCluster"].includes(
          spec.request.kind,
        )
      )
        window.dispatchEvent(new Event("orbwin:cluster-changed"));
      active.current = false;
      setPending(false);
      controller.current = null;
    }
  }
  return (
    <Context.Provider value={open}>
      {children}
      {forwardViews.length > 0 && (
        <aside className="port-forwards" aria-label="Port forwards">
          <details open>
            <summary>
              Port forwards ({forwardViews.filter((item) => item.active).length}{" "}
              active)
            </summary>
            <p>
              Localhost only. Forwards stay connected while navigating, for up
              to one hour. Closing OrbWin disconnects these clients.
            </p>
            {forwardViews.map((item) => (
              <div key={item.id}>
                <strong>{item.label}</strong>{" "}
                {item.active &&
                  !item.stopping &&
                  forwardUrls(item.output).map((port) => (
                    <LocalPortLink key={port.local} port={port.local}>
                      127.0.0.1:{port.local} → {port.remote}
                    </LocalPortLink>
                  ))}
                <span role="status">
                  {" "}
                  {item.stopping && item.active
                    ? "Disconnecting…"
                    : item.active
                      ? forwardUrls(item.output).length
                        ? "Connected"
                        : "Connecting…"
                      : item.message}
                </span>{" "}
                <button
                  className="button"
                  disabled={item.active && item.stopping}
                  onClick={() => {
                    if (item.active) {
                      setForwardViews((previous) =>
                        previous.map((entry) =>
                          entry.id === item.id
                            ? { ...entry, stopping: true }
                            : entry,
                        ),
                      );
                      forwards.current.get(item.id)?.abort();
                    } else
                      setForwardViews((previous) =>
                        previous.filter((entry) => entry.id !== item.id),
                      );
                  }}
                >
                  {item.active ? "Stop forward" : "Dismiss"}
                </button>
                <details>
                  <summary>Connection output</summary>
                  <pre className="command-output">
                    {item.output || item.message}
                  </pre>
                </details>
              </div>
            ))}
          </details>
        </aside>
      )}
      {spec && (
        <dialog
          ref={dialog}
          className="operation-dialog"
          aria-labelledby="operation-title"
          onCancel={(event) => {
            event.preventDefault();
            close();
          }}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            <h2 id="operation-title">{spec.title}</h2>
            {spec.description && <p>{spec.description}</p>}
            {spec.batch && (
              <ul className="operation-batch" aria-label="Affected resources">
                {spec.batch.map((item, index) => (
                  <li key={index}>{item.label}</li>
                ))}
              </ul>
            )}
            {spec.fields?.map((field) => (
              <label className="form-field" key={field.key}>
                {field.label}
                {field.multiline ? (
                  <textarea
                    disabled={pending}
                    value={values[field.key] ?? ""}
                    placeholder={field.placeholder}
                    onChange={(e) =>
                      setValues((previous) => ({
                        ...previous,
                        [field.key]: e.target.value,
                      }))
                    }
                  />
                ) : (
                  <input
                    required
                    disabled={pending}
                    value={values[field.key] ?? ""}
                    placeholder={field.placeholder}
                    onChange={(e) =>
                      setValues((previous) => ({
                        ...previous,
                        [field.key]: e.target.value,
                      }))
                    }
                  />
                )}
                {(field.key === "directory" || field.key === "file") && (
                  <button
                    type="button"
                    className="button"
                    disabled={pending}
                    onClick={() =>
                      void backend
                        .choosePath(field.key === "directory")
                        .then((path) => {
                          if (path)
                            setValues((previous) => ({
                              ...previous,
                              [field.key]: path,
                            }));
                        })
                        .catch((error) => setResult(String(error)))
                    }
                  >
                    Browse…
                  </button>
                )}
              </label>
            ))}
            {output && (
              <pre className="command-output" aria-label="Command output">
                {output}
              </pre>
            )}
            <p role="status" aria-live="polite">
              {pending ? "Operation in progress…" : result}
            </p>
            <div className="action-row">
              {!pending && !result && (
                <button
                  type="submit"
                  className={`button ${spec.destructive ? "danger" : ""}`}
                >
                  {spec.destructive ? "Confirm removal" : "Run"}
                </button>
              )}
              {!pending && output && (
                <button
                  type="button"
                  className="button"
                  onClick={() =>
                    void backend
                      .exportText(output)
                      .then((saved) => {
                        if (saved)
                          setResult("Output saved to the selected file.");
                      })
                      .catch((error) => setResult(String(error)))
                  }
                >
                  Save output…
                </button>
              )}
              <button type="button" className="button" onClick={close}>
                {pending ? "Cancel command" : "Close"}
              </button>
            </div>
          </form>
        </dialog>
      )}
    </Context.Provider>
  );
}

export function runContainerSpec(image = ""): Spec {
  return {
    title: "Run container",
    defaults: image ? { image } : undefined,
    request: { kind: "run", image: "", name: "", command: [] },
    description:
      "Starts a detached container on the OrbWin engine. Published TCP and UDP services are accessible from Windows at 127.0.0.1. LAN and IPv6 publication are not supported yet. Named volumes persist after container removal.",
    fields: [
      { key: "image", label: "Image reference" },
      { key: "name", label: "Container name" },
      {
        key: "command",
        label: "Optional command: executable, then one argument per line",
        multiline: true,
      },
      {
        key: "ports",
        label:
          "Published ports, one per line: 8080:80 or 80 for a dynamic localhost port",
        multiline: true,
      },
      {
        key: "environment",
        label: "Environment, one NAME=value per line",
        multiline: true,
      },
      {
        key: "volumes",
        label:
          "Mounts, one name:/container/path[:ro] or C:\\existing path:/container/path[:ro] per line",
        multiline: true,
      },
    ],
  };
}

export function ResourceCreate({
  page,
  disabled,
}: {
  page: ResourcePage;
  disabled: boolean;
}) {
  const open = useContext(Context);
  return (
    <button
      className="button"
      disabled={disabled}
      onClick={() =>
        open(
          page === "images"
            ? {
                title: "Pull image",
                request: { kind: "pull", image: "" },
                fields: [
                  {
                    key: "image",
                    label: "Image reference",
                    placeholder: "redis:alpine",
                  },
                ],
              }
            : page === "containers"
              ? runContainerSpec()
              : {
                  title:
                    page === "volumes" ? "Create volume" : "Create network",
                  request: {
                    kind: page === "volumes" ? "createVolume" : "createNetwork",
                    name: "",
                  },
                  fields: [{ key: "name", label: "Name" }],
                },
        )
      }
    >
      {page === "images" ? "Pull" : page === "containers" ? "Run" : "Create"}
    </button>
  );
}

export function ProjectControls({ disabled }: { disabled: boolean }) {
  const open = useContext(Context);
  return (
    <details className="project-controls">
      <summary>Compose project</summary>
      <p>
        Use an explicit project name and Compose file. Relative bind paths
        currently require the qualified WSL path form (BL-005). Down retains
        volumes.
      </p>
      <div className="action-row">
        {(
          ["up", "start", "stop", "restart", "pull", "logs", "down"] as const
        ).map((action) => (
          <button
            key={action}
            className="button"
            disabled={disabled}
            onClick={() =>
              open({
                title: `Compose ${action}`,
                request: { kind: "compose", file: "", project: "", action },
                description:
                  action === "down"
                    ? "Stops and removes this project's containers and networks. Volumes are retained. Container writable layers are removed."
                    : "Runs the selected Compose file on the OrbWin engine. The file may build images, pull images and configure workloads; select a project you trust.",
                fields: [
                  { key: "file", label: "Absolute Compose file path" },
                  { key: "project", label: "Project name" },
                ],
                destructive: action === "down",
              })
            }
          >
            {action}
          </button>
        ))}
      </div>
    </details>
  );
}
export function BuildControls({ disabled }: { disabled: boolean }) {
  const open = useContext(Context);
  return (
    <>
      <p>
        Build a project with its Dockerfile using the OrbWin engine's default
        builder. Output is streamed below the form; a successful build loads the
        image into this engine.
      </p>
      <button
        className="button"
        disabled={disabled}
        onClick={() =>
          open({
            title: "Build image",
            request: { kind: "build", directory: "", tag: "" },
            fields: [
              { key: "directory", label: "Absolute build context directory" },
              { key: "tag", label: "Image tag" },
            ],
            description:
              "Build context files are sent to the local OrbWin builder. Dockerfile steps execute container commands and may access the network. Cancelling the client may leave cached layers.",
          })
        }
      >
        Build image
      </button>
    </>
  );
}
const resourceKinds = {
  containers: "container",
  images: "image",
  volumes: "volume",
  networks: "network",
} as const;
export function resourceTarget(page: ResourcePage, row: backend.Resource) {
  return String(page === "volumes" ? row.Name : row.ID);
}
export type ContainerAction =
  | "start"
  | "stop"
  | "restart"
  | "pause"
  | "unpause";
function resourceTitle(row: backend.Resource) {
  return String(row.Names ?? row.Repository ?? row.Name ?? row.ID);
}
export function containerActionSpec(
  action: ContainerAction,
  row: backend.Resource,
): Spec {
  return {
    title: `${action[0].toUpperCase()}${action.slice(1)} ${resourceTitle(row)}`,
    request: {
      kind: "container",
      action,
      target: resourceTarget("containers", row),
    },
    description:
      action === "stop" || action === "restart"
        ? "This interrupts the selected container's workload. Volumes are retained."
        : "Applies only to the selected container on the OrbWin engine.",
  };
}
export function removalRequest(
  page: ResourcePage,
  row: backend.Resource,
): backend.Operation {
  return page === "images" &&
    row.Repository &&
    row.Repository !== "<none>" &&
    row.Tag &&
    row.Tag !== "<none>"
    ? { kind: "untag", reference: `${row.Repository}:${row.Tag}` }
    : {
        kind: "remove",
        resource: resourceKinds[page],
        target: resourceTarget(page, row),
      };
}
export const removalDescriptions: Record<ResourcePage, string> = {
  volumes:
    "Permanently deletes this volume and all of its data. This cannot be undone. Attached volumes are rejected by the engine.",
  containers:
    "Removes the stopped container and its writable layer. Named and anonymous volumes are retained. Running containers are rejected; stop them first.",
  images:
    "Removes the selected image tag, or an untagged image by ID. The underlying image can be removed when its last reference is deleted. No force removal is used.",
  networks:
    "Removes the selected network. The engine rejects networks still in use.",
};
export function removalSpec(page: ResourcePage, row: backend.Resource): Spec {
  return {
    title: `Remove ${resourceTitle(row)}?`,
    request: removalRequest(page, row),
    destructive: true,
    description: removalDescriptions[page],
  };
}
export function ResourceActions({
  page,
  row,
}: {
  page: ResourcePage;
  row: backend.Resource;
}) {
  const open = useContext(Context);
  const resource = resourceKinds[page];
  const target = resourceTarget(page, row);
  const title = resourceTitle(row);
  return (
    <div className="action-row" aria-label="Resource actions">
      {page === "containers" &&
        (["start", "stop", "restart", "pause", "unpause"] as const).map(
          (action) => (
            <button
              className="button"
              key={action}
              disabled={Boolean(containerActionUnavailable(action, row))}
              title={containerActionUnavailable(action, row)}
              onClick={() => open(containerActionSpec(action, row))}
            >
              {action === "unpause"
                ? "Resume"
                : action[0].toUpperCase() + action.slice(1)}
            </button>
          ),
        )}
      <button
        className="button"
        onClick={() =>
          open({
            title: `Inspect ${title}`,
            request: { kind: "inspect", resource, target },
          })
        }
      >
        Full inspect
      </button>
      {page === "containers" && (
        <button
          className="button"
          disabled={!String(row.State).match(/^running$/i)}
          title={
            String(row.State).match(/^running$/i)
              ? undefined
              : "Run commands require a running, unpaused container."
          }
          onClick={() =>
            open({
              title: `Run command in ${title}`,
              request: { kind: "exec", target, command: [] },
              fields: [
                {
                  key: "command",
                  label: "Executable, then one argument per line",
                  multiline: true,
                },
              ],
              description:
                "Runs inside the selected container. Cancellation stops the client; the exec process may remain in the container.",
            })
          }
        >
          Run command
        </button>
      )}
      {page === "images" && (
        <button
          className="button"
          onClick={() =>
            open({
              title: `Tag ${title}`,
              request: { kind: "tag", target, tag: "" },
              fields: [{ key: "tag", label: "New image reference" }],
            })
          }
        >
          Tag
        </button>
      )}
      {page === "networks" &&
        [false, true].map((disconnect) => (
          <button
            key={String(disconnect)}
            className="button"
            onClick={() =>
              open({
                title: `${disconnect ? "Disconnect from" : "Connect to"} ${title}`,
                request: {
                  kind: "connectNetwork",
                  network: target,
                  container: "",
                  disconnect,
                },
                fields: [{ key: "container", label: "Container full ID" }],
                description:
                  "Changes connectivity for the selected container. Existing application connections may be interrupted.",
              })
            }
          >
            {disconnect ? "Disconnect container" : "Connect container"}
          </button>
        ))}
      <button
        className="button danger"
        disabled={
          page === "containers" && Boolean(containerRemovalUnavailable(row))
        }
        title={
          page === "containers" ? containerRemovalUnavailable(row) : undefined
        }
        onClick={() => open(removalSpec(page, row))}
      >
        Remove
      </button>
    </div>
  );
}

export function ContainerView({
  target,
  tab,
}: {
  target: string;
  tab: "Logs" | "Stats";
}) {
  const [output, setOutput] = useState("");
  const [message, setMessage] = useState("");
  const [pending, setPending] = useState(false);
  const [query, setQuery] = useState("");
  const [matchingOnly, setMatchingOnly] = useState(false);
  const [wrapLogs, setWrapLogs] = useState(false);
  const [live, setLive] = useState(true);
  const [revision, setRevision] = useState(0);
  const [follow, setFollow] = useState(true);
  const controller = useRef<AbortController | null>(null);
  const active = useRef(false);
  async function execute(request: backend.Operation) {
    if (active.current) return;
    active.current = true;
    setPending(true);
    if (tab === "Logs") setOutput("");
    setMessage("");
    const abort = new AbortController();
    controller.current = abort;
    let first = true;
    try {
      const outcome = await backend.run(
        crypto.randomUUID(),
        request,
        (chunk) => {
          if (!abort.signal.aborted) {
            const initial = first;
            first = false;
            setOutput((previous) =>
              ((initial ? "" : previous) + chunk).slice(-131072),
            );
          }
        },
        abort.signal,
      );
      if (!abort.signal.aborted) setMessage(outcome.message);
    } catch (error) {
      if (!abort.signal.aborted) setMessage(String(error));
    } finally {
      active.current = false;
      setPending(false);
    }
  }
  useEffect(() => {
    const timer = window.setTimeout(() => {
      void execute(
        tab === "Logs"
          ? { kind: "logs", target, follow }
          : { kind: "stats", target },
      );
    }, 0);
    return () => {
      window.clearTimeout(timer);
      controller.current?.abort();
    };
  }, [target, tab, revision, follow]);
  useEffect(() => {
    if (tab !== "Stats" || !live) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible")
        void execute({ kind: "stats", target });
    }, 3000);
    return () => window.clearInterval(timer);
  }, [target, tab, live]);
  let stats: Record<string, unknown> | null = null;
  if (tab === "Stats") {
    try {
      stats = JSON.parse(output);
    } catch {
      /* A streamed JSON sample may be incomplete until its next chunk. */
    }
  }
  const visible =
    query && matchingOnly
      ? output
          .split("\n")
          .filter((line) => line.toLowerCase().includes(query.toLowerCase()))
          .join("\n")
      : output;
  // Escape the query: log search is literal text, never a user-supplied regex
  // or HTML. Captured separators retain the original output and casing.
  const fragments =
    tab === "Logs" && query
      ? visible.split(
          new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "gi"),
        )
      : [visible];
  return (
    <section className="container-view">
      <div className="action-row">
        {tab === "Logs" && (
          <label>
            <input
              type="checkbox"
              checked={follow}
              disabled={pending}
              onChange={(e) => setFollow(e.target.checked)}
            />{" "}
            Follow
          </label>
        )}
        {tab === "Stats" && (
          <label>
            <input
              type="checkbox"
              checked={live}
              onChange={(e) => setLive(e.target.checked)}
            />{" "}
            Refresh every 3 seconds
          </label>
        )}
        {output && (
          <button
            type="button"
            className="button"
            onClick={() =>
              void backend
                .exportText(visible)
                .then((saved) => {
                  if (saved) setMessage("Output saved to the selected file.");
                })
                .catch((error) => setMessage(String(error)))
            }
          >
            Save visible output…
          </button>
        )}
        <button
          className="button"
          disabled={pending}
          onClick={() => setRevision((n) => n + 1)}
        >
          {tab === "Stats" ? "Read current stats" : "Read logs"}
        </button>
      </div>
      {pending && (
        <button
          className="button"
          onClick={() => {
            controller.current?.abort();
            setLive(false);
            setMessage(
              "Client cancellation requested. Container processes and workloads may continue.",
            );
          }}
        >
          Stop client
        </button>
      )}
      <p role="status">
        {pending ? "Reading output…" : message || "No output received."}
      </p>
      {tab === "Logs" && (
        <label className="form-field">
          Search retained logs
          <input
            maxLength={256}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
      )}
      {tab === "Logs" && (
        <div className="action-row">
          <label>
            <input
              type="checkbox"
              checked={matchingOnly}
              onChange={(e) => setMatchingOnly(e.target.checked)}
            />{" "}
            Only matching lines
          </label>
          <label>
            <input
              type="checkbox"
              checked={wrapLogs}
              onChange={(e) => setWrapLogs(e.target.checked)}
            />{" "}
            Wrap lines
          </label>
        </div>
      )}
      {tab === "Logs" && query && (
        <small>
          {Math.floor((fragments.length - 1) / 2)} matches in retained logs.
        </small>
      )}
      {stats && typeof stats === "object" && !Array.isArray(stats) ? (
        <dl className="detail-properties">
          {Object.entries(stats).map(([key, value]) => (
            <div key={key}>
              <dt>{key}</dt>
              <dd>{String(value)}</dd>
            </div>
          ))}
        </dl>
      ) : (
        <pre
          className={`command-output ${tab === "Logs" && wrapLogs ? "wrap-log-lines" : ""}`}
        >
          {visible
            ? fragments.map((fragment, index) =>
                index % 2 === 1 ? (
                  <mark key={index}>{fragment}</mark>
                ) : (
                  fragment
                ),
              )
            : query && matchingOnly
              ? "No matching retained log lines."
              : ""}
        </pre>
      )}
      <small>
        Latest 128 KiB shown. Logs start with the last 500 lines; follow
        sessions are bounded to ten minutes.
      </small>
    </section>
  );
}
