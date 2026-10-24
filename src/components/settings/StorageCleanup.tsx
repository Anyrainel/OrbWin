import { useMemo, useRef, useState } from "react";
import * as backend from "../../backend";
import { ConfirmDialog } from "../shell/ConfirmDialog";
import { useToast } from "../shell/Toasts";
import {
  customNetworks,
  danglingImages,
  formatSize,
  imageSizeTotal,
  stoppedContainers,
  unattachedVolumes,
  type Candidate,
  type CleanupKind,
} from "./storage-model";

const resourceKind = {
  containers: "container",
  images: "image",
  networks: "network",
  volumes: "volume",
} as const;
const sections: {
  kind: CleanupKind;
  title: string;
  description: string;
  consequence: string;
  empty: string;
}[] = [
  {
    kind: "containers",
    title: "Stopped containers",
    description:
      "Exited, created or dead containers. Kubernetes-managed containers are excluded.",
    consequence:
      "Each container and its writable layer is removed. Named and anonymous volumes are retained.",
    empty: "No stopped containers in the current snapshot.",
  },
  {
    kind: "images",
    title: "Dangling images",
    description:
      "Untagged images with no repository. The engine rejects images still used by a container; no force removal is used.",
    consequence:
      "The selected untagged images are removed. Rebuilding or pulling may be needed to recreate them.",
    empty: "No dangling images in the current snapshot.",
  },
  {
    kind: "networks",
    title: "User-created networks",
    description:
      "Built-in bridge, host and none networks are excluded. Network usage is not part of the inventory snapshot, so the engine decides: networks still in use are rejected.",
    consequence:
      "The selected networks are removed. Containers attached to them would lose that connectivity; the engine rejects networks in use.",
    empty: "No user-created networks.",
  },
  {
    kind: "volumes",
    title: "Unattached volumes",
    description:
      "Volumes not mounted by any listed container, including stopped ones. The engine still rejects volumes in use.",
    consequence:
      "The selected volumes and all of their data are permanently deleted. This cannot be undone. Back up anything you need first.",
    empty: "No unattached volumes in the current snapshot.",
  },
];
type Result = { target: string; label: string; ok: boolean; message: string };

// Docker-data cleanup without a broad prune: the native adapter deliberately
// has no prune (src-tauri/src/operations.rs). Each selected item is removed
// with the existing single-resource operation, one at a time, after a
// per-category confirmation. Cancel stops after the current item; an
// interrupted removal may still have committed, so the inventory is re-read.
export function StorageCleanup({
  inventory,
  disabled,
  onMutation,
}: {
  inventory: backend.Inventory | null;
  disabled: boolean;
  onMutation: () => Promise<void>;
}) {
  const notify = useToast();
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [confirm, setConfirm] = useState<CleanupKind | null>(null);
  const [acknowledged, setAcknowledged] = useState(false);
  const [progress, setProgress] = useState<{
    kind: CleanupKind;
    done: number;
    total: number;
  } | null>(null);
  const [results, setResults] = useState<
    Partial<Record<CleanupKind, Result[]>>
  >({});
  const controller = useRef<AbortController | null>(null);
  const candidates = useMemo<Record<CleanupKind, Candidate[]>>(
    () => ({
      containers: stoppedContainers(inventory?.containers ?? []),
      images: danglingImages(inventory?.images ?? []),
      networks: customNetworks(inventory?.networks ?? []),
      volumes: unattachedVolumes(
        inventory?.volumes ?? [],
        inventory?.containers ?? [],
      ),
    }),
    [inventory],
  );
  const key = (item: Candidate) => `${item.kind}:${item.target}`;
  const chosen = (kind: CleanupKind) =>
    candidates[kind].filter((item) => selected[key(item)]);
  const size = imageSizeTotal(inventory?.images ?? []);

  async function remove(kind: CleanupKind) {
    const items = chosen(kind);
    if (!items.length || controller.current) return;
    const abort = new AbortController();
    controller.current = abort;
    const outcome: Result[] = [];
    setProgress({ kind, done: 0, total: items.length });
    try {
      for (const item of items) {
        if (abort.signal.aborted) break;
        try {
          const reply = await backend.run(
            crypto.randomUUID(),
            {
              kind: "remove",
              resource: resourceKind[kind],
              target: item.target,
            },
            () => {},
            abort.signal,
          );
          outcome.push({
            target: item.target,
            label: item.label,
            ok: reply.success,
            message: reply.success ? "Removed" : reply.message,
          });
        } catch (error) {
          outcome.push({
            target: item.target,
            label: item.label,
            ok: false,
            message: String(error),
          });
        }
        setProgress({ kind, done: outcome.length, total: items.length });
      }
    } finally {
      controller.current = null;
      setResults((previous) => ({ ...previous, [kind]: outcome }));
      setSelected((previous) => {
        const next = { ...previous };
        for (const item of outcome)
          if (item.ok) delete next[`${kind}:${item.target}`];
        return next;
      });
      await onMutation();
      setProgress(null);
      const removed = outcome.filter((item) => item.ok).length;
      const failed = outcome.length - removed;
      const skipped = items.length - outcome.length;
      notify(
        failed ? "error" : removed ? "success" : "info",
        `${removed} of ${items.length} ${kind} removed` +
          (failed ? `, ${failed} rejected or failed` : "") +
          (skipped ? `, ${skipped} not attempted after cancel` : "") +
          ". See Storage settings for details.",
      );
    }
  }

  return (
    <>
      <h2>Disk usage</h2>
      {inventory ? (
        <dl className="properties">
          <dt>Containers</dt>
          <dd>
            {inventory.containers.length} total, {candidates.containers.length}{" "}
            stopped
          </dd>
          <dt>Images</dt>
          <dd>
            {inventory.images.length} images, {formatSize(size.total)} reported
            {size.unknown ? ` (${size.unknown} without a size)` : ""}. Layers
            shared between images are counted once per image, so this overstates
            disk use.
          </dd>
          <dt>Volumes</dt>
          <dd>
            {inventory.volumes.length} volumes, {candidates.volumes.length}{" "}
            unattached. Volume sizes are not read yet.
          </dd>
          <dt>Networks</dt>
          <dd>{inventory.networks.length} networks</dd>
          <dt>Virtual disk</dt>
          <dd>
            The size of the OrbWin guest disk on Windows is not measured yet.
            Removing resources frees space inside the guest; the Windows virtual
            disk file may not shrink.
          </dd>
        </dl>
      ) : (
        <p className="notice">
          Disk usage needs a connected engine. Start the engine, then refresh.
        </p>
      )}
      <h2>Clean up unused resources</h2>
      <p>
        Select items in one category, then confirm that category separately.
        There is no single “remove everything” action, and no force removal.
      </p>
      {progress && (
        <div className="cleanup-progress" role="status">
          Removing {progress.kind}: {progress.done} of {progress.total}
          <progress value={progress.done} max={progress.total} />
          <button
            className="button compact"
            onClick={() => controller.current?.abort()}
          >
            Stop after current item
          </button>
        </div>
      )}
      {sections.map((section) => {
        const items = candidates[section.kind];
        const picked = chosen(section.kind);
        const all = items.length > 0 && picked.length === items.length;
        const sectionResults = results[section.kind];
        return (
          <section
            key={section.kind}
            className="cleanup-section"
            aria-labelledby={`cleanup-${section.kind}`}
          >
            <header>
              <div>
                <h3 id={`cleanup-${section.kind}`}>
                  {section.title} <small>({items.length})</small>
                </h3>
                <p>{section.description}</p>
              </div>
              <button
                className={`button ${section.kind === "volumes" ? "danger" : ""}`}
                disabled={disabled || !picked.length || progress !== null}
                onClick={() => {
                  setAcknowledged(false);
                  setConfirm(section.kind);
                }}
              >
                Remove {picked.length || ""} selected…
              </button>
            </header>
            {items.length ? (
              <ul className="cleanup-list">
                <li>
                  <label>
                    <input
                      type="checkbox"
                      checked={all}
                      disabled={disabled || progress !== null}
                      onChange={(event) =>
                        setSelected((previous) => {
                          const next = { ...previous };
                          for (const item of items)
                            next[key(item)] = event.target.checked;
                          return next;
                        })
                      }
                    />{" "}
                    Select all {items.length}
                  </label>
                </li>
                {items.map((item) => (
                  <li key={key(item)}>
                    <label>
                      <input
                        type="checkbox"
                        checked={Boolean(selected[key(item)])}
                        disabled={disabled || progress !== null}
                        onChange={(event) =>
                          setSelected((previous) => ({
                            ...previous,
                            [key(item)]: event.target.checked,
                          }))
                        }
                      />{" "}
                      <span>{item.label}</span>
                      {item.detail && <small>{item.detail}</small>}
                    </label>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="cleanup-empty">
                {inventory ? section.empty : "Unavailable without an engine."}
              </p>
            )}
            {sectionResults && sectionResults.some((item) => !item.ok) && (
              <ul className="cleanup-results" aria-label="Items not removed">
                {sectionResults
                  .filter((item) => !item.ok)
                  .map((item) => (
                    <li key={item.target}>
                      <strong>{item.label}</strong>: {item.message}
                    </li>
                  ))}
              </ul>
            )}
          </section>
        );
      })}
      {confirm && (
        <ConfirmDialog
          title={`Remove ${chosen(confirm).length} ${sections.find((s) => s.kind === confirm)!.title.toLowerCase()}?`}
          confirmLabel={confirm === "volumes" ? "Delete volumes" : "Remove"}
          destructive
          confirmDisabled={confirm === "volumes" && !acknowledged}
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            if (confirm === "volumes" && !acknowledged) return;
            const kind = confirm;
            setConfirm(null);
            void remove(kind);
          }}
        >
          <p>{sections.find((s) => s.kind === confirm)!.consequence}</p>
          <ul className="confirm-list">
            {chosen(confirm)
              .slice(0, 12)
              .map((item) => (
                <li key={item.target}>{item.label}</li>
              ))}
            {chosen(confirm).length > 12 && (
              <li>and {chosen(confirm).length - 12} more</li>
            )}
          </ul>
          {confirm === "volumes" && (
            <label className="confirm-ack">
              <input
                type="checkbox"
                checked={acknowledged}
                onChange={(event) => setAcknowledged(event.target.checked)}
              />{" "}
              I understand the volume data is permanently deleted.
            </label>
          )}
        </ConfirmDialog>
      )}
    </>
  );
}
