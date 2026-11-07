import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { useOperation } from "../Operations";
import type { VolumeFileEdit } from "./VolumeFileEditor";
type Entry = {
  name: string;
  kind: string;
  size: number;
  mode: number;
  link?: string;
};
type Snapshot = {
  entries: Entry[];
  complete: boolean;
  text?: string;
  sha256?: string;
  editSource?: { volume?: string; path?: string; reason?: string };
};

// The native client reads Docker archives without extracting or running code.
// Navigation aborts stale reads; only the current request may publish a result.
export function FilesPanel({
  target,
  resource = "container",
  onEditFile,
}: {
  target: string;
  resource?: "container" | "image" | "volume";
  onEditFile: (context: VolumeFileEdit, refreshPreview: () => void) => void;
}) {
  const operation = useOperation();
  const [path, setPath] = useState("/");
  const [location, setLocation] = useState("");
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [nativeBusy, setNativeBusy] = useState(false);
  const [nativeMessage, setNativeMessage] = useState("");
  const pending = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  const previewEpoch = useRef(0);
  const requester = useRef({ target, resource });
  if (
    requester.current.target !== target ||
    requester.current.resource !== resource
  )
    requester.current = { target, resource };
  const editVolume =
    resource === "volume" ? target : snapshot?.editSource?.volume;
  const editPath =
    resource === "volume" ? location : snapshot?.editSource?.path;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      pending.current?.abort();
    };
  }, []);
  async function read(selected: string, action: "list" | "read") {
    previewEpoch.current++;
    pending.current?.abort();
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    setSnapshot(null);
    setPath(selected);
    let output = "",
      diagnostic = "";
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "files", resource, target, path: selected, action },
        (chunk, stream) => {
          if (stream === "stderr")
            diagnostic = (diagnostic + chunk).slice(-8192);
          else output += chunk;
        },
        abort.signal,
      );
      if (abort.signal.aborted) return;
      if (!result.success) throw new Error(diagnostic || result.message);
      const value = JSON.parse(output) as Snapshot;
      if (!Array.isArray(value.entries) || typeof value.complete !== "boolean")
        throw new Error("Invalid file response.");
      setSnapshot(value);
      setLocation(selected);
    } catch (reason) {
      if (!abort.signal.aborted) setError(String(reason));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  const child = (name: string) => `${location.replace(/\/$/, "")}/${name}`;
  async function openCopy(edit: boolean) {
    setNativeBusy(true);
    setNativeMessage("");
    try {
      const opened = await backend.openSelectedCopy(edit);
      setNativeMessage(
        opened
          ? edit
            ? "Opened the selected local copy in Notepad. Saving there does not upload changes to the engine."
            : "Revealed the selected local file in Explorer."
          : "File selection cancelled.",
      );
    } catch (reason) {
      setNativeMessage(String(reason));
    } finally {
      setNativeBusy(false);
    }
  }
  return (
    <section className="files-panel">
      <h3>
        {resource === "container"
          ? "Container"
          : resource === "image"
            ? "Image"
            : "Volume"}{" "}
        files
      </h3>
      <p>
        {resource === "container"
          ? "Browse through Docker without a shell, including stopped containers. Browsing does not copy files onto Windows."
          : "Browse through a disposable pinned Alpine tools container with no network. The selected image never starts; volume data is mounted read-only. The bundled tools image loads offline when needed. Browsing does not copy files onto Windows. Links are displayed but cannot be followed in this view."}
      </p>
      <button
        className="button"
        disabled={busy}
        onClick={() =>
          operation({
            title: "Copy file to Windows",
            request: {
              kind: "copyFile",
              resource,
              target,
              path,
              destination: "",
            },
            description: `Copies the regular file at ${path} into a new Windows file without changing the source. Binary files up to 4 GiB are supported; links, directories and special files refuse. This is a live read, not a consistent backup. An exclusive partial file is published only after completion and flush. Existing destinations are never overwritten. Interrupted copies can leave a clearly named partial file.`,
            fields: [
              { key: "path", label: "Absolute Linux file path" },
              {
                key: "destination",
                label: "New Windows file destination",
                save: true,
              },
            ],
            defaults: { path },
          })
        }
      >
        Copy file to Windows…
      </button>
      <div className="action-row">
        <button
          className="button"
          disabled={nativeBusy}
          onClick={() => void openCopy(false)}
        >
          Reveal local copy…
        </button>
        <button
          className="button"
          disabled={nativeBusy}
          onClick={() => void openCopy(true)}
        >
          Edit local text copy…
        </button>
      </div>
      <p>
        After copying, select the local file to reveal it in Explorer or open
        UTF-8 text up to 256 KiB in Notepad. Local editor changes stay local.
        Use Edit text with backup for a guarded volume save.
      </p>
      {nativeMessage && <p role="status">{nativeMessage}</p>}
      <form
        className="action-row"
        onSubmit={(event) => {
          event.preventDefault();
          void read(path, "list");
        }}
      >
        <label>
          Linux path{" "}
          <input
            aria-label="Container file path"
            value={path}
            maxLength={2048}
            onChange={(event) => setPath(event.target.value)}
          />
        </label>
        <button className="button" disabled={busy}>
          Open directory
        </button>
        <button
          className="button"
          type="button"
          disabled={busy}
          onClick={() => void read(path, "read")}
        >
          Preview text
        </button>
        <button
          className="button"
          type="button"
          disabled={busy || !location || location === "/"}
          onClick={() =>
            void read(
              location.replace(/\/+$/, "").replace(/\/[^/]*$/, "") || "/",
              "list",
            )
          }
        >
          Parent directory
        </button>
      </form>
      {busy && (
        <div role="status">
          Reading files…{" "}
          <button
            className="button compact"
            onClick={() => {
              pending.current?.abort();
              setError("Read cancelled. Container files were not changed.");
            }}
          >
            Cancel
          </button>
        </div>
      )}
      {error && <p role="alert">{error}</p>}
      {!snapshot && !busy && !error && (
        <p>
          Enter a directory to browse, or a file path to preview UTF-8 text.
        </p>
      )}
      {snapshot && (
        <>
          <p role="status">
            {location}
            {!snapshot.complete &&
              " — Partial listing: reached the 8 MiB archive or 256-entry limit. Open a more specific directory."}
          </p>
          {editVolume &&
            editPath &&
            snapshot.text !== undefined &&
            snapshot.sha256 && (
              <button
                className="button"
                disabled={busy}
                onClick={() => {
                  const context = {
                    volume: editVolume,
                    path: editPath,
                    text: snapshot.text!,
                    version: snapshot.sha256!,
                  };
                  const selected = location;
                  const owner = requester.current;
                  const epoch = previewEpoch.current;
                  onEditFile(context, () => {
                    // Only this original preview may refresh after the editor closes.
                    if (
                      mounted.current &&
                      requester.current === owner &&
                      previewEpoch.current === epoch
                    )
                      void read(selected, "read");
                  });
                }}
              >
                Edit text with backup…
              </button>
            )}
          {resource === "container" &&
            snapshot.text !== undefined &&
            !editVolume && (
              <p className="capability-note">
                {snapshot.editSource?.reason ??
                  "Preview does not provide a verified writable volume source. Browse the intended volume directly for maintenance."}
              </p>
            )}
          {snapshot.text !== undefined ? (
            <pre
              className="file-preview"
              tabIndex={0}
              aria-label={`Text preview of ${location}`}
            >
              {snapshot.text || "(empty file)"}
            </pre>
          ) : snapshot.entries.length ? (
            <table className="usage-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Kind</th>
                  <th>Bytes</th>
                  <th>Mode</th>
                </tr>
              </thead>
              <tbody>
                {snapshot.entries.map((entry) => (
                  <tr key={entry.name}>
                    <td>
                      {entry.kind === "directory" || entry.kind === "file" ? (
                        <button
                          className="button compact"
                          onClick={() =>
                            void read(
                              child(entry.name),
                              entry.kind === "directory" ? "list" : "read",
                            )
                          }
                        >
                          {entry.name}
                        </button>
                      ) : (
                        entry.name
                      )}
                      {entry.link && <> → {entry.link}</>}
                    </td>
                    <td>{entry.kind}</td>
                    <td>{entry.size}</td>
                    <td>{entry.mode.toString(8)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <p>
              {snapshot.complete
                ? "This directory is empty."
                : "No direct entries were reached within the archive budget."}
            </p>
          )}
        </>
      )}
      <p className="capability-note">
        Listings are bounded reads of a live filesystem, not a backup or
        consistent snapshot. Text previews are limited to 256 KiB. Binary,
        oversized and special files report a clear preview limit. Copy file to
        Windows supports regular binary files up to 4 GiB.
      </p>
    </section>
  );
}
