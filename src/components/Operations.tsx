import { recheckWslDraft } from "../model/migration-wsl-draft";
import { invoke } from "@tauri-apps/api/core";
import {
  recheckMigrationDraft,
  type MigrationDraft,
} from "../model/migration-draft";
import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import * as backend from "../backend";
import { wslComposeRequest } from "../model/wsl-workflows";
import { containDialogTab } from "./shell/dialog-focus";
import { canReviewOperationFailure } from "./resources/operation-review";
import { ForwardEndpoint } from "./cluster/ForwardEndpoint";
import {
  containerActionUnavailable,
  containerRemovalUnavailable,
  parseContainerLimits,
  readResourceMetadata,
} from "./resources/model";
import type { ResourcePage } from "../model/navigation";
import { forwardUrls } from "./cluster/model";
import { clusterTarget, clusterDeletionTarget } from "./cluster/request-target";
import {
  composeInputKey,
  composeInputsProblem,
  loadComposeInputs,
  rememberComposeInputs,
  forgetComposeInputs,
  type ComposeInputs,
  type WslComposeInputs,
} from "../model/compose-inputs";

type Field = {
  key: string;
  label: string;
  multiline?: boolean;
  placeholder?: string;
  optional?: boolean;
  required?: boolean;
  save?: boolean;
  linux?: boolean;
  choices?: { value: string; label: string }[];
};
type Spec = {
  title: string;
  migration?: MigrationDraft;
  request: backend.Operation;
  description?: string;
  reviewDetails?: string;
  destructive?: boolean;
  confirmLabel?: string;
  returnFocus?: HTMLElement;
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
                ...clusterTarget(metadata),
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
              request: {
                kind: "clusterExec",
                ...clusterTarget(metadata),
                command: [],
              },
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
                  ? "Sets up the optional workload cluster in the owned OrbWin guest. Applying restarts that guest and interrupts its Docker and Kubernetes workloads; persistent data is retained. Downloads can take several minutes. Cancelling stops this client, but an accepted backend change can continue applying."
                  : "Disables the OrbWin workload cluster. Applying restarts the owned guest and interrupts its Docker and Kubernetes workloads; persistent data is retained. Other engines and global Kubernetes configuration are not changed. Cancelling stops this client, but an accepted backend change can continue applying.",
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
          request: {
            kind: "clusterDelete",
            resource,
            ...clusterDeletionTarget(metadata),
          },
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
  const [remember, setRemember] = useState(false);
  const [pending, setPending] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [reviewable, setReviewable] = useState(false);
  const [output, setOutput] = useState("");
  const [result, setResult] = useState("");
  const [copiedPath, setCopiedPath] = useState("");
  const [openingCopy, setOpeningCopy] = useState(false);
  const [networkChoices, setNetworkChoices] = useState<string[]>([]);
  const [networkReading, setNetworkReading] = useState(false);
  const [networkMessage, setNetworkMessage] = useState("");
  const networkRead = useRef<object | null>(null);
  useEffect(
    () => () => {
      networkRead.current = null;
    },
    [],
  );
  const dialog = useRef<HTMLDialogElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const active = useRef(false);
  const controller = useRef<AbortController | null>(null);
  const trigger = useRef<HTMLElement | null>(null);
  const open = (next: Spec) => {
    if (active.current) return;
    trigger.current =
      next.returnFocus ?? (document.activeElement as HTMLElement);
    setSpec(next);
    setValues(next.defaults ?? {});
    setRemember(false);
    setOutput("");
    setResult("");
    setReviewable(false);
    setStopping(false);
    setCopiedPath("");
    networkRead.current = null;
    setNetworkReading(false);
    setNetworkChoices([]);
    setNetworkMessage("");
  };
  async function readNetworks() {
    if (networkRead.current) return;
    const token = {};
    networkRead.current = token;
    setNetworkReading(true);
    setNetworkMessage("");
    try {
      const snapshot = await backend.inventory();
      if (networkRead.current !== token) return;
      const choices = [
        ...new Set(
          snapshot.networks
            .filter(
              (row) =>
                row.Driver === "bridge" &&
                row.Scope === "local" &&
                typeof row.Name === "string",
            )
            .map((row) => String(row.Name)),
        ),
      ].sort();
      if (choices.length > 512)
        throw new Error(
          "Too many networks to display; enter the intended existing bridge network name.",
        );
      setNetworkChoices(choices);
      setNetworkMessage(
        choices.length
          ? "Choose a current bridge network or enter its name. Docker verifies it when Run is submitted."
          : "No local bridge networks were returned. Refresh or create the intended network first.",
      );
    } catch (error) {
      if (networkRead.current === token) setNetworkMessage(String(error));
    } finally {
      if (networkRead.current === token) {
        networkRead.current = null;
        setNetworkReading(false);
      }
    }
  }
  useEffect(() => {
    if (spec) {
      dialog.current?.showModal();
      if (spec.destructive) closeButton.current?.focus();
    } else trigger.current?.focus();
  }, [spec]);
  function close() {
    if (active.current) {
      setStopping(true);
      controller.current?.abort();
      return;
    }
    networkRead.current = null;
    dialog.current?.close();
    setSpec(null);
  }
  async function submit() {
    if (!spec || active.current || result) return;
    active.current = true;
    setReviewable(false);
    let dispatched = false;
    let succeeded = false;
    let reconciled = true;
    let request = { ...spec.request };
    let mutation = false;
    try {
      for (const field of spec.fields ?? []) {
        const raw = values[field.key] ?? "";
        if (field.required && !raw.trim())
          throw new Error(`Complete ${field.label} before continuing.`);
        if (
          request.kind === "run" &&
          ["restart", "network", "cpus", "memoryMiB"].includes(field.key)
        ) {
          request = {
            ...request,
            options: { ...request.options, [field.key]: raw.trim() },
          };
          continue;
        }
        if (request.kind === "createNetwork" && field.key === "internal") {
          request = { ...request, internal: raw === "true" };
          continue;
        }
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
        if (
          (request.kind === "compose" || request.kind === "wslCompose") &&
          ["overrides", "envFiles", "profiles", "services"].includes(field.key)
        ) {
          request = {
            ...request,
            [field.key]: raw
              .split(/\r?\n/)
              .map((line) => line.trim())
              .filter(Boolean),
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
        dispatched = true;
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
      setCopiedPath("");
      const abort = new AbortController();
      controller.current = abort;
      const append = (chunk: string) =>
        setOutput((previous) => (previous + chunk).slice(-131072));
      if (
        spec.migration &&
        (request.kind === "compose" || request.kind === "wslCompose")
      ) {
        const draft = spec.migration;
        const readSource = async () => {
          let stdout = "",
            stderr = "",
            overflow = false;
          const reviewRequest: backend.Operation =
            draft.source.kind === "docker-desktop"
              ? { kind: "reviewDesktopMigration" }
              : {
                  kind: "reviewRancherMigration",
                  registration: draft.source.wsl!.registration,
                  machine: draft.source.wsl!.machine,
                };
          const checked = await backend.run(
            crypto.randomUUID(),
            reviewRequest,
            (chunk, stream) => {
              if (stream === "stdout") {
                if (stdout.length + chunk.length > 2 * 1024 * 1024)
                  overflow = true;
                else stdout += chunk;
              } else {
                if (stderr.length + chunk.length > 8192) overflow = true;
                else stderr += chunk;
              }
            },
            abort.signal,
          );
          if (!checked.success || overflow || abort.signal.aborted)
            throw Error(
              "Migration source recheck failed or exceeded its bounds. Refresh migration review; no Compose request was dispatched.",
            );
          return stdout;
        };
        if (request.kind === "compose") {
          await recheckMigrationDraft(
            draft,
            {
              file: request.file,
              overrides: request.overrides ?? [],
              directory: request.directory ?? "",
            },
            request.action,
            readSource,
            async (file, contents, target) => {
              await invoke("inspect_migration_mapping", {
                path: file.path,
                expectedSha: file.sha256,
                contents,
                target,
              });
            },
            abort.signal,
          );
        } else {
          if (!request.connection?.registration)
            throw Error(
              "Refresh the WSL project registration before reopening.",
            );
          const inputs: WslComposeInputs = {
            kind: "wsl",
            distribution: request.distribution,
            connection: {
              ...request.connection,
              registration: request.connection.registration,
            },
            project: request.project,
            file: request.file,
            overrides: request.overrides ?? [],
            envFiles: request.envFiles ?? [],
            profiles: request.profiles ?? [],
            services: request.services ?? [],
            directory: request.directory ?? "",
            migration: draft,
          };
          const mappingReview = await recheckWslDraft(
            inputs,
            request.action,
            () =>
              invoke("wsl_migration_project", {
                name: inputs.distribution,
                registration: inputs.connection.registration,
              }),
            readSource,
            abort.signal,
          );
          request = { ...request, reviewedDaemon: draft.target, mappingReview };
        }
      }
      if (spec.batch) {
        dispatched = true;
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
            mutation = true;
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
        dispatched = true;
        // A transport failure can follow acceptance; reconcile until a returned
        // outcome explicitly tells us this request did not mutate the engine.
        mutation = true;
        const outcome = await backend.run(
          crypto.randomUUID(),
          request,
          append,
          abort.signal,
        );
        mutation = outcome.mutation;
        succeeded = outcome.success;
        setResult(outcome.message);
        if (outcome.success && request.kind === "copyFile")
          setCopiedPath(request.destination);
        if (
          outcome.success &&
          (request.kind === "compose" ||
            (request.kind === "wslCompose" && !!spec.migration)) &&
          remember &&
          (!spec.migration || request.action === "validate")
        ) {
          try {
            const remembered: ComposeInputs = {
              project: request.project,
              file: request.file,
              overrides: request.overrides ?? [],
              envFiles: request.envFiles ?? [],
              profiles: request.profiles ?? [],
              services: request.services ?? [],
              directory: request.directory ?? "",
              ...(spec.migration
                ? { migration: structuredClone(spec.migration) }
                : {}),
            };
            rememberComposeInputs(
              request.kind === "wslCompose"
                ? {
                    ...remembered,
                    kind: "wsl",
                    distribution: request.distribution,
                    connection: {
                      ...request.connection!,
                      registration: request.connection!.registration!,
                    },
                  }
                : remembered,
            );
            window.dispatchEvent(new Event("orbwin:compose-inputs-changed"));
          } catch (error) {
            setResult(
              `${outcome.message}\nOperation succeeded, but inputs were not remembered: ${String(error)}`,
            );
          }
        }
      }
    } catch (error) {
      setResult(String(error));
    } finally {
      // Even a failed/cancelled mutation may have committed. Reconcile via reads.
      try {
        if (mutation) await onMutation();
      } catch (error) {
        reconciled = false;
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
      setReviewable(
        canReviewOperationFailure({
          dispatched,
          succeeded,
          cancelled: controller.current?.signal.aborted ?? false,
          reconciled,
        }),
      );
      setPending(false);
      setStopping(false);
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
                    <ForwardEndpoint
                      key={`${port.local}:${port.remote}`}
                      local={port.local}
                      remote={port.remote}
                    />
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
          onKeyDown={containDialogTab}
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
            {spec.reviewDetails && (
              <details className="operation-review-details">
                <summary>Source review and safety details</summary>
                <p>{spec.reviewDetails}</p>
              </details>
            )}
            {copiedPath && (
              <div
                className="action-row"
                aria-label="Open completed local copy"
              >
                {[false, true].map((edit) => (
                  <button
                    key={String(edit)}
                    type="button"
                    className="button"
                    disabled={openingCopy || pending}
                    onClick={async () => {
                      setOpeningCopy(true);
                      try {
                        await backend.openLocalCopy(copiedPath, edit);
                        setResult(
                          edit
                            ? "Opened the completed local copy in Notepad. Local changes are not uploaded to the engine."
                            : "Revealed the completed local copy in Explorer.",
                        );
                      } catch (error) {
                        setResult(String(error));
                      } finally {
                        setOpeningCopy(false);
                      }
                    }}
                  >
                    {edit ? "Edit copy in Notepad" : "Reveal copy in Explorer"}
                  </button>
                ))}
              </div>
            )}
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
                {field.choices ? (
                  <select
                    disabled={pending}
                    value={values[field.key] ?? ""}
                    onChange={(e) =>
                      setValues((previous) => ({
                        ...previous,
                        [field.key]: e.target.value,
                      }))
                    }
                  >
                    {field.choices.map((choice) => (
                      <option key={choice.value} value={choice.value}>
                        {choice.label}
                      </option>
                    ))}
                  </select>
                ) : field.multiline ? (
                  <textarea
                    required={field.required}
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
                    required={!field.optional}
                    disabled={pending}
                    value={values[field.key] ?? ""}
                    placeholder={field.placeholder}
                    list={
                      spec.request.kind === "run" && field.key === "network"
                        ? "run-network-choices"
                        : undefined
                    }
                    onChange={(e) =>
                      setValues((previous) => ({
                        ...previous,
                        [field.key]: e.target.value,
                      }))
                    }
                  />
                )}
                {spec.request.kind === "run" && field.key === "network" && (
                  <>
                    <datalist id="run-network-choices">
                      {networkChoices.map((network) => (
                        <option key={network} value={network} />
                      ))}
                    </datalist>
                    <button
                      type="button"
                      className="button"
                      disabled={pending || networkReading}
                      onClick={() => void readNetworks()}
                    >
                      {networkReading
                        ? "Reading networks…"
                        : "Read existing networks"}
                    </button>
                    {networkReading && (
                      <button
                        type="button"
                        className="button"
                        onClick={() => {
                          networkRead.current = null;
                          setNetworkReading(false);
                          setNetworkMessage(
                            "Network read cancelled; its result will be discarded.",
                          );
                        }}
                      >
                        Cancel network read
                      </button>
                    )}
                    <span role="status">{networkMessage}</span>
                  </>
                )}
                {!field.linux &&
                  (field.key === "directory" ||
                    field.key === "file" ||
                    field.key === "source" ||
                    field.save) && (
                    <button
                      type="button"
                      className="button"
                      disabled={pending}
                      onClick={() =>
                        void (
                          field.save
                            ? backend.chooseSavePath()
                            : backend.choosePath(field.key === "directory")
                        )
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
            {(spec.request.kind === "compose" ||
              (spec.request.kind === "wslCompose" && !!spec.migration)) &&
              (!spec.migration || spec.request.action === "validate") && (
                <label className="form-field compose-remember">
                  <span>
                    <input
                      type="checkbox"
                      checked={remember}
                      disabled={pending || !!result}
                      onChange={(event) => setRemember(event.target.checked)}
                    />{" "}
                    Remember these reviewed inputs on this device after success
                  </span>
                  <small>
                    Stores paths, selected profiles and migration provenance
                    hints only; no transfer status. Files and environment values
                    are read again on each run.
                  </small>
                </label>
              )}
            {output && (
              <pre className="command-output" aria-label="Command output">
                {output}
              </pre>
            )}
            <p role="status" aria-live="polite">
              {pending
                ? stopping
                  ? "Stopping the client… Accepted changes may remain. Waiting for the result and state refresh."
                  : "Operation in progress…"
                : result}
            </p>
            <div className="action-row">
              {!pending && result && reviewable && (
                <button
                  type="button"
                  className="button"
                  onClick={() => {
                    setResult("");
                    setReviewable(false);
                    dialog.current
                      ?.querySelector<
                        | HTMLInputElement
                        | HTMLSelectElement
                        | HTMLTextAreaElement
                      >("input:not(:disabled), select:not(:disabled), textarea:not(:disabled)")
                      ?.focus();
                  }}
                >
                  Review inputs
                </button>
              )}
              {!pending && !result && (
                <button
                  type="submit"
                  className={`button ${spec.destructive ? "danger" : ""}`}
                >
                  {spec.confirmLabel ??
                    (spec.destructive ? "Confirm removal" : "Run")}
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
              <button
                ref={closeButton}
                type="button"
                className="button"
                disabled={pending && stopping}
                onClick={close}
              >
                {pending
                  ? stopping
                    ? "Stopping client…"
                    : "Cancel command"
                  : "Close"}
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
      "Starts a detached container on the OrbWin engine. Optional CPU quota throttles work; it does not reserve CPUs. A hard RAM limit can cause out-of-memory termination; Docker's default swap policy still applies. Blank resource fields leave Docker defaults. Choose an existing local network by name; blank uses Docker's default bridge. On failure restarts nonzero exits, unless stopped respects an explicit stop, and always can start again with the engine. Published TCP and UDP services are accessible from Windows at 127.0.0.1. LAN and IPv6 publication are not supported yet. Named volumes persist after container removal. Cancelling closes this client; an accepted container may remain. Inspect before retrying.",
    fields: [
      { key: "image", label: "Image reference" },
      { key: "name", label: "Container name" },
      {
        key: "cpus",
        label: "Optional CPU limit (0.01–1024, for example 0.5)",
        optional: true,
      },
      {
        key: "memoryMiB",
        label: "Optional hard RAM limit in MiB (whole number, minimum 6)",
        optional: true,
      },
      {
        key: "restart",
        label: "Restart policy",
        choices: [
          { value: "", label: "No automatic restart" },
          { value: "on-failure", label: "On failure" },
          { value: "unless-stopped", label: "Unless stopped" },
          { value: "always", label: "Always" },
        ],
      },
      {
        key: "network",
        label: "Existing network name (blank uses default bridge)",
        optional: true,
        placeholder: "my-project_default",
      },
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
    <>
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
                      kind:
                        page === "volumes" ? "createVolume" : "createNetwork",
                      name: "",
                    },
                    description:
                      page === "networks"
                        ? "Creates a bridge network. Overlay/Swarm and macvlan/ipvlan direct-IP or L2 routing are outside the supported Phase 2 scope. Host bypasses bridge isolation and is not offered. Optional IPv4 subnet must be canonical; Docker checks conflicts. Internal prevents external routing through this network. Cancellation may leave an accepted network; inspect before retrying."
                        : undefined,
                    fields:
                      page === "volumes"
                        ? [{ key: "name", label: "Name" }]
                        : [
                            { key: "name", label: "Name" },
                            {
                              key: "driver",
                              label: "Driver",
                              choices: [
                                { value: "", label: "Bridge (supported)" },
                              ],
                            },
                            {
                              key: "subnet",
                              label: "Optional IPv4 subnet",
                              optional: true,
                              placeholder: "172.30.0.0/24",
                            },
                            {
                              key: "internal",
                              label: "External routing",
                              choices: [
                                { value: "", label: "Allow external routing" },
                                {
                                  value: "true",
                                  label: "Internal network only",
                                },
                              ],
                            },
                          ],
                  },
          )
        }
      >
        {page === "images" ? "Pull" : page === "containers" ? "Run" : "Create"}
      </button>
      {page === "volumes" && (
        <button
          className="button"
          disabled={disabled}
          onClick={() =>
            open({
              title: "Restore portable volume data",
              confirmLabel: "Restore data",
              request: { kind: "restoreVolume", source: "", name: "" },
              destructive: true,
              description:
                "Validates the full versioned package and checksums before creating a new ordinary local volume. Existing volume names refuse. Content and metadata are checked again after import. Stop external clients from attaching or replacing the destination during restore. Failure/cancellation retains the source package and a pending destination with its ownership receipt; do not attach a pending volume until verification completes. This transfers selected volume data, not a whole engine, credentials or Kubernetes state.",
              fields: [
                {
                  key: "source",
                  label: "Portable package on a local Windows drive",
                },
                { key: "name", label: "New destination volume name" },
              ],
            })
          }
        >
          Restore data…
        </button>
      )}
    </>
  );
}

export function composeSpec(
  action: Extract<backend.Operation, { kind: "compose" }>["action"],
  inputs?: ComposeInputs,
  note = "",
): Spec {
  const spec: Spec = {
    title: `Compose ${action}${inputs ? ` ${inputs.project}` : ""}`,
    migration: inputs?.migration
      ? structuredClone(inputs.migration)
      : undefined,
    request: { kind: "compose", file: "", project: "", action },
    defaults: inputs
      ? {
          project: inputs.project,
          file: inputs.file,
          directory: inputs.directory,
          overrides: inputs.overrides.join("\n"),
          envFiles: (inputs.envFiles ?? []).join("\n"),
          profiles: inputs.profiles.join("\n"),
          services: (inputs.services ?? []).join("\n"),
        }
      : undefined,
    confirmLabel: action === "validate" ? "Validate" : undefined,
    description:
      action === "validate"
        ? "Check these project files without creating workloads. Review the inputs below before validating."
        : undefined,
    reviewDetails: `${inputs?.migration ? "Saved migration provenance is an unverified hint. Submission rereads the exact source/target and checks saved mapping bytes and semantics. No transfer status is saved. " : ""}${note} Review files, working directory, project name, profiles and services before running. Files are read anew; select files you trust. Compose can read local interpolation/include data. ${action === "down" ? "Removes the whole project's containers (including writable layers) and networks. Volumes are retained. Profiles do not restrict Down to those services; remembered service choices are not used." : action === "validate" ? "Checks the whole configuration without creating workloads. Configuration values are not printed; errors can contain file content. Validation does not reserve ports or guarantee startup." : action === "up" ? "Starts/reconciles selected services and their dependencies, and may recreate containers. An explicitly selected service can activate its profile. Cancellation stops the client; accepted changes may remain." : action === "logs" ? "Reads up to 500 log lines per selected service/container, with service prefixes. Blank selects all enabled services. This snapshot does not follow future output." : "Applies to selected services and their replicas using upstream Compose. Cancellation stops the client; accepted changes may remain."}`,
    fields: [
      { key: "file", label: "Absolute base Compose file path" },
      {
        key: "envFiles",
        label:
          "Interpolation env files in order (optional, one absolute path per line; later files override earlier ones)",
        multiline: true,
      },
      {
        key: "overrides",
        label:
          "Override file paths in merge order (optional, one absolute path per line)",
        multiline: true,
      },
      {
        key: "directory",
        label:
          "Project working directory (optional; defaults to the base file's directory)",
        optional: true,
      },
      {
        key: "profiles",
        label: "Profiles (optional, one name per line)",
        multiline: true,
      },
      { key: "project", label: "Project name" },
      ...(!["down", "validate"].includes(action)
        ? [
            {
              key: "services",
              label:
                "Services (optional, one exact Compose service name per line; blank selects all enabled services)",
              multiline: true,
            },
          ]
        : []),
    ],
    destructive: action === "down",
  };
  if (action !== "validate") {
    spec.description = spec.reviewDetails;
    spec.reviewDetails = undefined;
  }
  return spec;
}
export function wslComposeSpec(
  distribution: string,
  source: string,
  action: Extract<backend.Operation, { kind: "wslCompose" }>["action"],
  inputs?: Partial<Extract<backend.Operation, { kind: "wslCompose" }>> & {
    signature?: string;
    migration?: MigrationDraft;
  },
): Spec {
  const request = wslComposeRequest(distribution, source, action, inputs);
  const spec: Spec = {
    title: `${distribution}: Compose ${action}`,
    migration: inputs?.migration
      ? structuredClone(inputs.migration)
      : undefined,
    request,
    defaults: {
      file: inputs?.file ?? "compose.yaml",
      project: request.project,
      directory: inputs?.directory ?? "",
      overrides: (inputs?.overrides ?? []).join("\n"),
      envFiles: (inputs?.envFiles ?? []).join("\n"),
      profiles: (inputs?.profiles ?? []).join("\n"),
      services: (inputs?.services ?? []).join("\n"),
    },
    confirmLabel: action === "validate" ? "Validate" : undefined,
    description:
      action === "validate"
        ? "Check these Linux project files and supported paths without creating workloads. Review the inputs below before validating."
        : undefined,
    reviewDetails: `Uses the recorded project ${source} as this distribution's normal user. Review original Linux paths or paths relative to the selected project root. Existing bind/build/file-backed sources must stay inside that root; paths map automatically to the engine. Selected profiles/services and dependencies follow Compose rules. ${action === "down" ? "Down removes the whole project's containers and networks, retaining volumes." : action === "validate" ? "Validates resolved configuration and supported paths without printing configuration values." : "Accepted engine changes may continue after client cancellation."} Keep a WSL session open. A disconnected/replaced source refuses until explicitly reconnected. No global Docker context or shell configuration changes.`,
    destructive: action === "down",
    fields: [
      {
        key: "file",
        label: "Base Compose file (Linux project path)",
        linux: true,
      },
      {
        key: "envFiles",
        label:
          "Interpolation env files in order (optional, one Linux project path per line; later files override earlier ones)",
        multiline: true,
        linux: true,
      },
      {
        key: "overrides",
        label:
          "Override files in merge order (optional, one Linux project path per line)",
        multiline: true,
        linux: true,
      },
      {
        key: "directory",
        label: "Working directory (optional; selected project root by default)",
        optional: true,
        linux: true,
      },
      { key: "project", label: "Explicit project name" },
      {
        key: "profiles",
        label: "Profiles (optional, one per line)",
        multiline: true,
      },
      ...(!["validate", "down"].includes(action)
        ? [
            {
              key: "services",
              label:
                "Exact services (optional, one per line; blank selects enabled services)",
              multiline: true,
            },
          ]
        : []),
    ],
  };
  if (action !== "validate") {
    spec.description = spec.reviewDetails;
    spec.reviewDetails = undefined;
  }
  return spec;
}

/** Short picker text; exact stored identity is available in the selected details. */
export function rememberedProjectLabel(item: ComposeInputs | WslComposeInputs) {
  const source = item.migration
    ? item.migration.source.kind === "docker-desktop"
      ? "Docker Desktop"
      : "Rancher Desktop"
    : "";
  const compact = (value: string, limit: number) =>
    value.length > limit ? `${value.slice(0, limit - 1)}…` : value;
  return [
    compact(item.project, 36),
    source,
    item.kind === "wsl" ? `WSL: ${compact(item.distribution, 24)}` : "",
  ]
    .filter(Boolean)
    .join(" · ");
}

export function RememberedProjectDetails({
  inputs,
}: {
  inputs: ComposeInputs | WslComposeInputs;
}) {
  return (
    <details className="operation-review-details">
      <summary>Saved project details</summary>
      <dl>
        <dt>Project</dt>
        <dd>{inputs.project}</dd>
        <dt>Base file</dt>
        <dd>{inputs.file}</dd>
        {inputs.kind === "wsl" && (
          <>
            <dt>WSL distribution</dt>
            <dd>{inputs.distribution}</dd>
            <dt>Connected source</dt>
            <dd>{inputs.connection.source}</dd>
            <dt>Registration</dt>
            <dd>{inputs.connection.registration}</dd>
            <dt>Root identity</dt>
            <dd>{inputs.connection.signature}</dd>
          </>
        )}
        {inputs.migration && (
          <>
            <dt>Migration source</dt>
            <dd>{inputs.migration.source.kind}</dd>
            <dt>Source project</dt>
            <dd>{inputs.migration.source.project}</dd>
            <dt>Source daemon</dt>
            <dd>{inputs.migration.source.daemon}</dd>
            <dt>Target daemon</dt>
            <dd>{inputs.migration.target}</dd>
            {inputs.migration.source.wsl && (
              <>
                <dt>Source WSL registration</dt>
                <dd>{inputs.migration.source.wsl.registration}</dd>
                <dt>Source WSL machine</dt>
                <dd>{inputs.migration.source.wsl.machine}</dd>
              </>
            )}
          </>
        )}
      </dl>
      {inputs.migration && (
        <p>
          Saved provenance is an unverified hint. Validation rereads the source,
          target and saved mapping; transfer status comes from owning receipts.
        </p>
      )}
    </details>
  );
}

export function ProjectControls({ disabled }: { disabled: boolean }) {
  const open = useContext(Context);
  const [saved, setSaved] = useState(loadComposeInputs);
  const [selected, setSelected] = useState("");
  const [error, setError] = useState(composeInputsProblem);
  useEffect(() => {
    const reload = () => {
      setSaved(loadComposeInputs());
      setError(composeInputsProblem());
    };
    window.addEventListener("orbwin:compose-inputs-changed", reload);
    return () =>
      window.removeEventListener("orbwin:compose-inputs-changed", reload);
  }, []);
  const inputs = saved.find((item) => composeInputKey(item) === selected);
  return (
    <details className="project-controls">
      <summary>Compose project</summary>
      <p>
        Review ordered files and optional profiles. Validate checks
        configuration without creating workloads. Remembered inputs are editable
        hints; the engine remains authoritative.
      </p>
      <label className="form-field">
        Remembered project
        <select
          value={inputs ? selected : ""}
          disabled={disabled}
          onChange={(event) => setSelected(event.target.value)}
        >
          <option value="">New project inputs</option>
          {saved.map((item) => (
            <option key={composeInputKey(item)} value={composeInputKey(item)}>
              {rememberedProjectLabel(item)}
            </option>
          ))}
        </select>
      </label>
      {inputs && <RememberedProjectDetails inputs={inputs} />}
      {inputs && (
        <button
          className="button"
          disabled={disabled}
          onClick={() => {
            try {
              forgetComposeInputs(composeInputKey(inputs));
              setSaved(loadComposeInputs());
              setSelected("");
              setError("");
            } catch (error) {
              setError(String(error));
            }
          }}
        >
          Forget remembered inputs
        </button>
      )}
      {error && <p role="alert">{error}</p>}
      <div className="action-row">
        {(inputs?.migration
          ? (["validate", "up"] as const)
          : ([
              "validate",
              "up",
              "start",
              "stop",
              "restart",
              "pull",
              "logs",
              "down",
            ] as const)
        ).map((action) => (
          <button
            key={action}
            className="button"
            disabled={disabled}
            onClick={() =>
              open(
                inputs?.kind === "wsl"
                  ? wslComposeSpec(
                      inputs.distribution,
                      inputs.connection.source,
                      action,
                      {
                        ...inputs,
                        kind: "wslCompose",
                        migration: inputs.migration,
                      },
                    )
                  : composeSpec(action, inputs),
              )
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
export function ContainerResourceLimits({ row }: { row: backend.Resource }) {
  const open = useOperation();
  const pendingRead = useRef<AbortController | null>(null);
  const currentRow = useRef(row);
  currentRow.current = row;
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [review, setReview] = useState<{
    row: backend.Resource;
    limits: ReturnType<typeof parseContainerLimits>;
    observedAt: string;
  } | null>(null);
  useEffect(() => {
    pendingRead.current?.abort();
    setMessage("");
    return () => pendingRead.current?.abort();
  }, [row]);
  const current = review?.row === row ? review.limits : undefined;
  async function read() {
    if (pendingRead.current) return;
    const abort = new AbortController();
    pendingRead.current = abort;
    setBusy(true);
    setMessage("");
    setReview(null);
    try {
      const limits = await readResourceMetadata(
        (output, signal) =>
          backend.run(
            crypto.randomUUID(),
            { kind: "inspect", resource: "container", target: String(row.ID) },
            output,
            signal,
          ),
        abort.signal,
        () => currentRow.current === row,
        (output) => parseContainerLimits(output, String(row.ID)),
      );
      if (!limits || abort.signal.aborted || currentRow.current !== row) return;
      setReview({
        row,
        limits,
        observedAt: new Date().toLocaleString(),
      });
      setMessage(
        "Configuration read. Enforcement still depends on the engine and kernel; this does not measure available capacity.",
      );
    } catch (error) {
      if (!abort.signal.aborted) setMessage(String(error));
    } finally {
      if (pendingRead.current === abort) {
        pendingRead.current = null;
        setBusy(false);
        if (abort.signal.aborted && currentRow.current === row)
          setMessage("Limit read cancelled.");
      }
    }
  }
  return (
    <section className="info-section" aria-label="Container resource limits">
      <h3>Container resource limits</h3>
      <button className="button" disabled={busy} onClick={() => void read()}>
        {busy ? "Reading limits…" : "Read current limits"}
      </button>
      {busy && (
        <button className="button" onClick={() => pendingRead.current?.abort()}>
          Cancel limit read
        </button>
      )}
      <p role="status">
        {message ||
          "Read this container's current Docker settings before reviewing a change."}
      </p>
      {current && (
        <>
          <p>
            Configuration observed {review?.observedAt}. Refresh before relying
            on a later change.
          </p>
          <dl className="detail-properties">
            <div>
              <dt>NanoCPUs</dt>
              <dd>{current.nanoCpus ?? "Unknown"}</dd>
            </div>
            <div>
              <dt>CFS quota / period (µs)</dt>
              <dd>
                {current.cpuQuota ?? "Unknown"} /{" "}
                {current.cpuPeriod ?? "Unknown"}
              </dd>
            </div>
            <div>
              <dt>Hard RAM limit (bytes)</dt>
              <dd>
                {current.memory === 0
                  ? "No explicit limit (0)"
                  : (current.memory ?? "Unknown")}
              </dd>
            </div>
            <div>
              <dt>RAM + swap setting (bytes)</dt>
              <dd>
                {current.memorySwap === -1
                  ? "Unlimited swap (-1)"
                  : current.memorySwap === 0
                    ? "Docker default (0)"
                    : (current.memorySwap ?? "Unknown")}
              </dd>
            </div>
          </dl>
          <button
            className="button"
            onClick={() =>
              open({
                title: `Change limits for ${resourceTitle(row)}`,
                request: {
                  kind: "updateContainerLimits",
                  target: String(row.ID),
                  cpus: "",
                  memoryMiB: "",
                },
                confirmLabel: "Apply limits",
                destructive: true,
                description:
                  "Applies positive CPU and/or hard RAM limits to this exact container ID using Docker update, without restarting it. Blank fields leave that setting unchanged; zero/removing limits is not offered here. Lowering RAM can kill processes or the workload; CPU throttling can cause timeouts. Existing swap policy is retained and may make a RAM update refuse. Other clients may change settings after this review; submitted fields replace their latest values. Compose may replace these settings on recreation; edit the project's source for lasting configuration. Cancellation can leave accepted changes applied; refresh and inspect before retrying.",
                fields: [
                  {
                    key: "cpus",
                    label: "New CPU limit (0.01–1024, blank leaves unchanged)",
                    optional: true,
                  },
                  {
                    key: "memoryMiB",
                    label:
                      "New hard RAM limit in MiB (minimum 6, blank leaves unchanged)",
                    optional: true,
                  },
                ],
              })
            }
          >
            Change limits…
          </button>
          <p>
            Zero CPU quota fields mean no quota was reported through that field;
            CPU sets and other scheduler constraints can still apply. These are
            per-container settings, not independent WSL or OrbWin VM allocation.
          </p>
        </>
      )}
    </section>
  );
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
