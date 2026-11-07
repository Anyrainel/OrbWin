import { useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import * as backend from "../../backend";
import { parseMigrationWSLSources } from "../../model/migration-images";
import {
  migrationWslReview,
  parseProjectConnection,
  type ProjectConnection,
  appendWslMapping,
  parseWslMappingSave,
} from "../../model/migration-wsl";
import type { migrationProjectReview } from "../../model/migration-projects";

export function MigrationWslProject({
  review,
  onReview,
  mappingContents,
}: {
  review: ReturnType<typeof migrationProjectReview>;
  onReview: (prepared: ReturnType<typeof migrationWslReview>) => void;
  mappingContents: string;
}) {
  const [choices, setChoices] = useState<{ id: string; name: string }[] | null>(
    null,
  );
  const [selected, setSelected] = useState("");
  const [connection, setConnection] = useState<ProjectConnection | null>(null);
  const [filename, setFilename] = useState(
    () => `orbwin-volume-map-${crypto.randomUUID().slice(0, 8)}.compose.json`,
  );
  const [staged, setStaged] = useState<{
    path: string;
    sha256: string;
    contents: string;
    connection: ProjectConnection;
    filename: string;
  } | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const epoch = useRef(0),
    pending = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      ++epoch.current;
      pending.current?.abort();
    },
    [],
  );
  async function inspect(list: boolean) {
    const mine = ++epoch.current;
    setBusy(true);
    setError("");
    setConnection(null);
    setStaged(null);
    try {
      if (list) {
        const abort = new AbortController();
        pending.current = abort;
        let output = "";
        const result = await backend.run(
          crypto.randomUUID(),
          { kind: "migrationWslSources" },
          (text, stream) => {
            if (stream === "stdout") {
              output += text;
              if (output.length > 65536) abort.abort();
            }
          },
          abort.signal,
        );
        if (mine !== epoch.current) return;
        if (!result.success || abort.signal.aborted)
          throw new Error(
            result.message || "Could not list project distributions.",
          );
        setChoices(parseMigrationWSLSources(output));
        setSelected("");
      } else {
        const choice = choices?.find((item) => item.id === selected);
        if (!choice)
          throw new Error("Select the distribution that owns your project.");
        const status = await invoke("wsl_migration_project", {
          name: choice.name,
          registration: choice.id,
        });
        if (mine === epoch.current)
          setConnection(parseProjectConnection(status, choice));
      }
    } catch (error) {
      if (mine === epoch.current) setError(String(error));
    } finally {
      if (mine === epoch.current) {
        setBusy(false);
        pending.current = null;
      }
    }
  }
  const prepared = connection ? migrationWslReview(review, connection) : null;
  const currentMapping =
    staged &&
    connection &&
    staged.contents === mappingContents &&
    staged.filename === filename &&
    staged.connection.registration === connection.registration &&
    staged.connection.source === connection.source &&
    staged.connection.signature === connection.signature
      ? staged
      : null;
  async function saveMapping() {
    if (!connection || !mappingContents || busy) return;
    const mine = ++epoch.current,
      selected = connection,
      contents = mappingContents,
      name = filename;
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    setStaged(null);
    let text = "",
      diagnostic = "",
      overflow = false;
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        {
          kind: "wslSaveMapping",
          distribution: selected.name,
          connection: {
            registration: selected.registration,
            source: selected.source,
            signature: selected.signature,
          },
          target: review.target,
          file: name,
          contents,
        },
        (chunk, stream) => {
          if (stream === "stderr")
            diagnostic = (diagnostic + chunk).slice(-8192);
          else if (text.length + chunk.length > 65536) overflow = true;
          else text += chunk;
        },
        abort.signal,
      );
      if (mine !== epoch.current) return;
      if (!result.success || abort.signal.aborted || overflow)
        throw new Error(
          `${result.message} ${diagnostic} Inspect ${selected.source}/${name} before retrying; a new file may remain.`,
        );
      const saved = parseWslMappingSave(text, selected, name);
      setStaged({ ...saved, contents, connection: selected, filename: name });
    } catch (reason) {
      if (mine === epoch.current) setError(String(reason));
    } finally {
      if (mine === epoch.current) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  function continueReview() {
    if (
      !prepared ||
      !connection ||
      busy ||
      (mappingContents && !currentMapping)
    )
      return;
    try {
      onReview(
        currentMapping
          ? appendWslMapping(
              prepared,
              connection,
              currentMapping.path,
              currentMapping.sha256,
            )
          : prepared,
      );
    } catch (reason) {
      setError(String(reason));
    }
  }
  return (
    <details>
      <summary>Use a connected WSL project</summary>
      <p>
        Choose the distribution that owns the files. This is independent of the
        source engine’s socket connection. Inspection may start the selected
        distro; it checks an existing project export and never connects or
        replaces one. Keep a WSL session open.
      </p>
      <button
        className="button"
        disabled={busy}
        onClick={() => void inspect(true)}
      >
        {choices
          ? "Refresh project distributions"
          : "List project distributions"}
      </button>
      {choices && (
        <label>
          Project distribution
          <select
            disabled={busy}
            value={selected}
            onChange={(event) => {
              setSelected(event.target.value);
              setConnection(null);
              setError("");
            }}
          >
            <option value="">Choose explicitly</option>
            {choices.map((choice) => (
              <option key={choice.id} value={choice.id}>
                {choice.name} · {choice.id}
              </option>
            ))}
          </select>
        </label>
      )}
      {choices?.length === 0 && (
        <p>No eligible ordinary WSL 2 distributions were found.</p>
      )}
      <button
        className="button"
        disabled={busy || !selected}
        onClick={() => void inspect(false)}
      >
        Inspect existing project connection
      </button>
      {busy && <p role="status">Reading or saving the selected project…</p>}
      {busy && pending.current && (
        <button className="button" onClick={() => pending.current?.abort()}>
          Cancel read/save
        </button>
      )}
      {error && <p role="alert">{error}</p>}
      {prepared && connection && (
        <>
          <p role="status">
            Verified project: {connection.name} ·{" "}
            <code>{connection.source}</code>
          </p>
          <p>{prepared.message}</p>
          {mappingContents && (
            <section aria-label="Save volume mapping in WSL project">
              <p>
                Save the reviewed volume mapping as a new file inside this
                original project. Existing files are never replaced.
                Cancellation can leave a new or partial file; inspect its name
                before retrying.
              </p>
              <label>
                New mapping filename in project root
                <input
                  value={filename}
                  disabled={busy}
                  onChange={(event) => {
                    setFilename(event.target.value);
                    setStaged(null);
                    setError("");
                  }}
                />
              </label>
              <p>
                Destination:{" "}
                <code>
                  {connection.source}/{filename}
                </code>
              </p>
              <button
                className="button"
                disabled={busy}
                onClick={() => void saveMapping()}
              >
                Save mapping in connected project
              </button>
              {currentMapping && (
                <p role="status">
                  Saved <code>{currentMapping.path}</code>. It will be appended
                  last in Linux validation. SHA-256:{" "}
                  <code>{currentMapping.sha256}</code>.
                </p>
              )}
              {staged && !currentMapping && (
                <p role="status">
                  The saved file belongs to an earlier draft or connection. Save
                  the current mapping before continuing.
                </p>
              )}
            </section>
          )}
          <button
            className="button"
            disabled={busy || (!!mappingContents && !currentMapping)}
            onClick={continueReview}
          >
            Review Linux Compose files…
          </button>
        </>
      )}
    </details>
  );
}
