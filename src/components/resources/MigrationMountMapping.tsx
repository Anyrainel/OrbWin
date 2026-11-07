import { useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import {
  reviewMigrationMounts,
  migrationMountDraft,
} from "../../model/migration-mounts";
import type { MigrationProjectMember } from "../../model/migration-images";
import { MigrationDestinationReview } from "./MigrationDestinationReview";

export function MigrationMountMapping({
  members,
  targetVolumes,
  target,
  source,
  onSaved,
  mapping,
  namespace,
  onMappingChange,
}: {
  members: MigrationProjectMember[];
  targetVolumes: string[];
  target: string;
  source: string;
  onSaved: (path: string, contents: string, sha256: string) => void;
  mapping: Record<string, string>;
  namespace: string;
  onMappingChange: (mapping: Record<string, string>) => void;
}) {
  const [busy, setBusy] = useState(false),
    [message, setMessage] = useState("");
  const preview = useRef<HTMLTextAreaElement>(null);
  const codeReview = useRef<HTMLDetailsElement>(null);
  const generation = useRef(0);
  useEffect(
    () => () => {
      ++generation.current;
    },
    [],
  );
  let mounts: ReturnType<typeof reviewMigrationMounts> | undefined = undefined;
  let problem = "",
    draft = "";
  try {
    mounts = reviewMigrationMounts(members);
  } catch (error) {
    problem = String(error).replace(/^Error: /, "");
  }
  if (mounts?.length) {
    try {
      draft = migrationMountDraft(mounts, mapping, namespace, targetVolumes);
    } catch (error) {
      problem = String(error).replace(/^Error: /, "");
    }
  }
  async function save() {
    if (!draft || busy) return;
    const mine = ++generation.current;
    setBusy(true);
    setMessage("");
    try {
      const saved = await invoke<{
        path: string;
        sha256: string;
        target: string;
      } | null>("save_migration_mapping", {
        contents: draft,
        target,
      });
      if (mine !== generation.current) return;
      if (
        saved &&
        (saved.target !== target ||
          !/^[a-f0-9]{64}$/.test(saved.sha256) ||
          typeof saved.path !== "string")
      )
        throw Error(
          "Saved mapping response is invalid; review the file again.",
        );
      if (saved) onSaved(saved.path, draft, saved.sha256);
      setMessage(
        saved
          ? `Saved ${saved.path}. Windows project validation will append this override last. Review the complete file order and Validate before any separately reviewed Up. For Linux files, use Save mapping in connected project under the WSL project review. No workload was started.`
          : "Save cancelled; no file was created.",
      );
    } catch (error) {
      if (mine === generation.current) setMessage(String(error));
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <details>
      <summary>Map restored volumes to service mount paths</summary>
      <p>
        Export and restore each selected volume first, then select its existing
        OrbWin destination here. Use the explicit receipt check for recorded
        source restore provenance; current data consistency still needs review.
        Bind files, secrets/configs, environment files, ports and profiles still
        require separate review.
      </p>
      {mounts?.length === 0 && (
        <p>No ordinary named-volume service mounts were observed.</p>
      )}
      {mounts &&
        [...new Set(mounts.map((mount) => mount.sourceVolume))].map(
          (volume) => (
            <div key={volume}>
              <label>
                Source volume <code>{volume}</code> → OrbWin volume
                <select
                  value={mapping[volume] ?? ""}
                  disabled={busy}
                  onChange={(event) => {
                    onMappingChange({
                      ...mapping,
                      [volume]: event.target.value,
                    });
                    setMessage("");
                  }}
                >
                  <option value="">Choose a restored volume</option>
                  {targetVolumes.map((name) => (
                    <option key={name} value={name}>
                      {name}
                    </option>
                  ))}
                </select>
              </label>
              <MigrationDestinationReview
                key={`${source}:${target}:${volume}:${mapping[volume] ?? ""}`}
                source={source}
                target={target}
                sourceVolume={volume}
                destination={mapping[volume] ?? ""}
              />
            </div>
          ),
        )}
      {mounts && (
        <ul>
          {mounts.map((mount) => (
            <li key={`${mount.service}:${mount.target}`}>
              <code>{mount.service}</code>: <code>{mount.target}</code> ←{" "}
              <code>{mount.sourceVolume}</code>
              {mount.readOnly ? " (read-only)" : " (writable)"}
            </li>
          ))}
        </ul>
      )}
      {problem && <p role="status">{problem}</p>}
      {draft && (
        <>
          <details ref={codeReview}>
            <summary>Review Compose override JSON</summary>
            <label className="migration-map-preview">
              Compose volume mapping draft
              <textarea ref={preview} readOnly rows={12} value={draft} />
            </label>
          </details>
          <div className="action-row">
            <button
              className="button"
              onClick={() => {
                if (codeReview.current) codeReview.current.open = true;
                preview.current?.focus();
                preview.current?.select();
              }}
            >
              Select draft
            </button>
            <button
              className="button"
              disabled={busy}
              onClick={() => void save()}
            >
              {busy ? "Saving…" : "Save new override…"}
            </button>
          </div>
          <p>
            Only the listed service mounts are replaced. Destinations are
            external volumes and image data is not copied into them. Save uses a
            new Windows JSON file and refuses overwrite. For Linux validation,
            save the current draft through the connected WSL project review.
            Source files are retained.
          </p>
        </>
      )}
      {message && <p role="status">{message}</p>}
    </details>
  );
}
