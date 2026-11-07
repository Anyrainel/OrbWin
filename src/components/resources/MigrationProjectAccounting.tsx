import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import {
  migrationAccounting,
  readMigrationAccountingHistories,
  type MigrationAccountingProject,
} from "../../model/migration-accounting";

// Historical evidence only. The owning SDK receipts remain the sole records;
// reopening this reviewer rereads them rather than restoring a synthetic job.
export function MigrationProjectAccounting({
  project,
  mapping,
  busy: parentBusy,
}: {
  project: MigrationAccountingProject;
  mapping: Record<string, string>;
  busy: boolean;
}) {
  const [histories, setHistories] = useState<Awaited<
    ReturnType<typeof readMigrationAccountingHistories>
  > | null>(null);
  const [busy, setBusy] = useState(false),
    [message, setMessage] = useState(""),
    [error, setError] = useState("");
  const pending = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      pending.current?.abort();
      pending.current = null;
    },
    [],
  );
  async function read() {
    if (pending.current || parentBusy) return;
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setMessage("");
    setError("");
    setHistories(null);
    try {
      const result = await readMigrationAccountingHistories(
        project.target,
        async (kind) => {
          let output = "",
            diagnostic = "",
            overflow = false;
          const result = await backend.run(
            crypto.randomUUID(),
            { kind },
            (chunk, stream) => {
              if (stream === "stderr") {
                diagnostic = (diagnostic + chunk).slice(-8192);
                return;
              }
              if (output.length + chunk.length > 2 * 1024 * 1024) {
                overflow = true;
                abort.abort();
                return;
              }
              output += chunk;
            },
            abort.signal,
          );
          if (overflow)
            throw new Error(
              "Project receipt history exceeded its response budget. No partial accounting was published.",
            );
          if (!result.success) throw new Error(diagnostic || result.message);
          return output;
        },
        abort.signal,
      );
      if (pending.current !== abort || abort.signal.aborted) return;
      // Validate all identity relationships before publishing either history.
      migrationAccounting(project, result.images, result.volumes, mapping);
      setHistories(result);
      setMessage(
        `Historical receipts read at ${new Date().toLocaleTimeString()}.`,
      );
    } catch (error) {
      if (pending.current === abort) setError(String(error));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  let accounting: ReturnType<typeof migrationAccounting> | null = null,
    problem = "";
  if (histories) {
    try {
      accounting = migrationAccounting(
        project,
        histories.images,
        histories.volumes,
        mapping,
      );
    } catch (error) {
      problem = String(error);
    }
  }
  return (
    <details aria-busy={busy}>
      <summary>Project transfer receipts</summary>
      <p>
        Matches the explicitly observed source project{" "}
        <code>{project.inputs.project}</code> on {project.source.kind},{" "}
        <code>{project.source.daemon}</code> to owning transfer receipts for
        target <code>{project.target}</code>. A matching asset receipt does not
        prove it was exported for this project. No latest transaction is
        selected.
      </p>
      <button
        className="button"
        type="button"
        disabled={busy || parentBusy}
        onClick={() => void read()}
      >
        {busy ? "Reading both histories…" : "Read matching transfer receipts"}
      </button>{" "}
      {busy && (
        <button
          className="button"
          type="button"
          onClick={() => {
            pending.current?.abort();
            pending.current = null;
            setBusy(false);
            setMessage("Project receipt read cancelled. No data was changed.");
          }}
        >
          Cancel read
        </button>
      )}
      {message && <p role="status">{message}</p>}
      {busy && (
        <p role="status">Reading transfer receipts. No transfer is running.</p>
      )}
      {error && <p role="alert">{error}</p>}
      {problem && <p role="alert">{problem}</p>}
      {accounting && (
        <>
          <p>
            Historical acknowledgements only. Import/restore completion does not
            establish current tags, writable data, destination ownership, valid
            Compose files or a running migrated application. Check mapped
            destination ownership separately before use.
          </p>
          {accounting.uncertainImageReceipts +
            accounting.uncertainVolumeReceipts +
            accounting.unavailableImportReceipts >
            0 && (
            <p role="status">
              Unassociated or unreadable evidence:{" "}
              {accounting.uncertainImageReceipts} source-image receipts,{" "}
              {accounting.unavailableImportReceipts} import receipts and{" "}
              {accounting.uncertainVolumeReceipts} source-volume receipts. These
              cannot be assigned to this project; matching history may be
              incomplete.
            </p>
          )}
          {accounting.unknownImages > 0 && (
            <p role="status">
              {accounting.unknownImages} ordinary member(s) have no observed
              immutable image identity. No receipt association was guessed.
            </p>
          )}
          {!accounting.images.length && !accounting.volumes.length && (
            <p>
              No ordinary image or named-volume identities were observed for
              receipt matching.
            </p>
          )}
          {accounting.images.map((image) => (
            <section
              key={image.id}
              aria-label={`Historical image receipts ${image.id}`}
            >
              <p>
                <strong>{image.services.join(", ") || "Observed image"}</strong>
                : <code>{image.id}</code> · {image.exports.length} matching
                export receipt(s).
              </p>
              {!image.exports.length && (
                <p>
                  No matching receipt was found in this bounded history. This is
                  not proof that no transfer occurred.
                </p>
              )}
              {image.exports.slice(0, 8).map((row) => (
                <details key={row.transaction}>
                  <summary>
                    Export {row.transaction} ·{" "}
                    {row.exportAcknowledged
                      ? "acknowledged"
                      : "publication unconfirmed"}
                  </summary>
                  <p>
                    Source image identity is matched directly; archive config
                    IDs are separate. {row.imports?.length ?? 0} linked import
                    receipt(s).
                  </p>
                  {(row.imports ?? []).slice(0, 8).map((link) => (
                    <p key={link.transaction}>
                      <code>{link.transaction}</code> · {link.phase} ·{" "}
                      {link.verifiedConfigs} archive config(s) verified
                      historically
                    </p>
                  ))}
                  {(row.imports?.length ?? 0) > 8 && (
                    <p>
                      Showing 8 of {row.imports?.length} linked imports; inspect
                      the owning image history for all receipts.
                    </p>
                  )}
                </details>
              ))}
              {image.exports.length > 8 && (
                <p>
                  Showing 8 of {image.exports.length} exports in transaction
                  order, not newest-first; inspect the owning image history for
                  all receipts.
                </p>
              )}
            </section>
          ))}
          {accounting.volumes.map((volume) => (
            <section
              key={volume.name}
              aria-label={`Historical volume receipts ${volume.name}`}
            >
              <p>
                <strong>{volume.name}</strong> →{" "}
                {volume.selected || "No destination mapping selected"} ·{" "}
                {volume.exports.length} matching source export receipt(s).
              </p>
              {!volume.exports.length && (
                <p>
                  No matching receipt was found in this bounded history. Source
                  cleanup is not export acknowledgement.
                </p>
              )}
              {volume.exports
                .slice(0, 8)
                .map(({ receipt, matchingRestores }) => (
                  <details key={receipt.transaction}>
                    <summary>
                      Export {receipt.transaction} ·{" "}
                      {receipt.exportAcknowledged
                        ? "acknowledged"
                        : "publication unconfirmed"}{" "}
                      · tools {receipt.phase}
                    </summary>
                    <p>
                      {receipt.manifestSha256
                        ? "Full manifest provenance recorded."
                        : "No full manifest provenance; no stronger restore association was inferred."}{" "}
                      {matchingRestores.length} linked restore(s) for the
                      selected destination.
                    </p>
                    {matchingRestores.slice(0, 8).map((link) => (
                      <p key={link.transaction}>
                        <code>{link.transaction}</code> · {link.volume} ·{" "}
                        {link.phase}
                      </p>
                    ))}
                    {volume.selected &&
                      receipt.exportAcknowledged &&
                      !matchingRestores.length && (
                        <p>
                          No linked restore for this selected destination was
                          observed; naming or equal payload bytes alone do not
                          establish a link.
                        </p>
                      )}
                    {matchingRestores.length > 8 && (
                      <p>
                        Showing 8 of {matchingRestores.length} restores; inspect
                        owning source-volume history for all receipts.
                      </p>
                    )}
                  </details>
                ))}
              {volume.exports.length > 8 && (
                <p>
                  Showing 8 of {volume.exports.length} exports in transaction
                  order, not newest-first; inspect owning source-volume history
                  for all receipts.
                </p>
              )}
            </section>
          ))}
        </>
      )}
    </details>
  );
}
