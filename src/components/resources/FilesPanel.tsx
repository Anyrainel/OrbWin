import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
type Entry = {
  name: string;
  kind: string;
  size: number;
  mode: number;
  link?: string;
};
type Snapshot = { entries: Entry[]; complete: boolean; text?: string };

// The native client reads Docker archives without extracting or running code.
// Navigation aborts stale reads; only the current request may publish a result.
export function FilesPanel({ target }: { target: string }) {
  const [path, setPath] = useState("/");
  const [location, setLocation] = useState("");
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  async function read(selected: string, action: "list" | "read") {
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
        { kind: "files", target, path: selected, action },
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
  return (
    <section className="files-panel">
      <h3>Container files</h3>
      <p>
        Read-only access through Docker. Works without a shell, including
        stopped containers. Nothing is extracted onto Windows.
      </p>
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
        oversized and special files report a clear limit. Use the explicitly
        targeted Docker CLI for larger copies.
      </p>
    </section>
  );
}
