import { useEffect, useRef, useState } from "react";
import * as backend from "../backend";
import { BuildControls } from "./Operations";

type Build = {
  ref: string;
  name?: string;
  status?: string;
  created_at?: string;
  completed_at?: string;
  cached_steps?: number;
  total_steps?: number;
};
// BuildKit owns retained history. UI selection/output are disposable snapshots;
// no app-owned build database or synthetic history is introduced.
export function BuildHistory({ connected }: { connected: boolean }) {
  const [builds, setBuilds] = useState<Build[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [details, setDetails] = useState("");
  const [tab, setTab] = useState<"inspect" | "logs">("inspect");
  const pending = useRef<AbortController | null>(null);
  const detailPending = useRef<AbortController | null>(null);
  async function refresh() {
    if (!connected) return;
    pending.current?.abort();
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    let text = "";
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "buildHistory", action: "list", reference: "" },
        (chunk, stream) => {
          if (stream !== "stderr") text += chunk;
        },
        abort.signal,
      );
      if (abort.signal.aborted) return;
      if (!result.success) throw new Error(`${result.message}\n${text}`);
      const rows = text
        .split(/\r?\n/)
        .filter((line) => line.trim())
        .map((line) => JSON.parse(line) as Build);
      if (rows.some((row) => typeof row.ref !== "string" || !row.ref))
        throw new Error(
          "The backend returned a build record without a reference.",
        );
      setBuilds(rows);
      setSelected((prior) =>
        rows.some((row) => row.ref === prior) ? prior : null,
      );
    } catch (reason) {
      if (!abort.signal.aborted) {
        setError(String(reason));
        setBuilds([]);
        setSelected(null);
      }
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  useEffect(() => {
    if (!connected) {
      pending.current?.abort();
      pending.current = null;
      setBusy(false);
      setBuilds([]);
      setSelected(null);
      return;
    }
    void refresh();
    return () => {
      pending.current?.abort();
      detailPending.current?.abort();
    };
  }, [connected]);
  useEffect(() => {
    detailPending.current?.abort();
    setDetails("");
    if (!selected || !connected) return;
    const abort = new AbortController();
    detailPending.current = abort;
    let text = "";
    setDetails("Reading build record…");
    void backend
      .run(
        crypto.randomUUID(),
        { kind: "buildHistory", action: tab, reference: selected },
        (chunk, stream) => {
          if (tab === "logs" || stream !== "stderr") text += chunk;
          if (!abort.signal.aborted) setDetails(text);
        },
        abort.signal,
      )
      .then((result) => {
        if (!abort.signal.aborted) {
          if (!result.success) setDetails(`${result.message}\n${text}`);
          else if (tab === "inspect") {
            try {
              setDetails(JSON.stringify(JSON.parse(text), null, 2));
            } catch {
              setDetails(
                "Invalid build detail response. Refresh the history before retrying.",
              );
            }
          }
        }
      })
      .catch((reason) => {
        if (!abort.signal.aborted) setDetails(String(reason));
      });
    return () => abort.abort();
  }, [connected, selected, tab]);
  const filtered = builds.filter((build) =>
    `${build.name} ${build.ref} ${build.status}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  return (
    <>
      <div className="action-row">
        <button
          className="button"
          disabled={!connected || busy}
          onClick={() => void refresh()}
        >
          {busy ? "Reading history…" : "Refresh history"}
        </button>
        {busy && (
          <button className="button" onClick={() => pending.current?.abort()}>
            Cancel read
          </button>
        )}
      </div>
      {!connected ? (
        <p className="notice">
          Connect the OrbWin engine to read its build history.
        </p>
      ) : error ? (
        <p role="alert">{error}</p>
      ) : (
        <>
          <label>
            Search builds{" "}
            <input
              className="input"
              aria-label="Search build history"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          {!busy && filtered.length === 0 && (
            <p>
              {query
                ? "No matching builds."
                : "No retained builds in this engine. Build a project to create a record."}
            </p>
          )}
          <div className="build-history-layout">
            <div
              className="build-history-list"
              role="list"
              aria-label="Retained builds"
            >
              {filtered.map((build) => (
                <button
                  className={`resource-row ${selected === build.ref ? "selected" : ""}`}
                  aria-pressed={selected === build.ref}
                  key={build.ref}
                  onClick={() => setSelected(build.ref)}
                >
                  <span>
                    <strong>{build.name || build.ref}</strong>
                    <small>
                      {build.status ?? "Unknown status"} ·{" "}
                      {build.created_at
                        ? new Date(build.created_at).toLocaleString()
                        : "Time unavailable"}
                    </small>
                  </span>
                </button>
              ))}
            </div>
            <section aria-label="Build details">
              <div className="action-row">
                <button
                  className="button"
                  disabled={!selected}
                  aria-pressed={tab === "inspect"}
                  onClick={() => setTab("inspect")}
                >
                  Details
                </button>
                <button
                  className="button"
                  disabled={!selected}
                  aria-pressed={tab === "logs"}
                  onClick={() => setTab("logs")}
                >
                  Steps and logs
                </button>
              </div>
              <pre className="command-output" aria-live="polite">
                {selected
                  ? details
                  : "Select a build to inspect its steps and output."}
              </pre>
            </section>
          </div>
        </>
      )}
      <BuildControls disabled={!connected} />
    </>
  );
}
