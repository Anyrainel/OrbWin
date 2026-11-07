import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import {
  engineProxyConfig,
  engineProxyReviewCurrent,
  readEngineProxySnapshot,
  type EngineProxyConfig,
  type EngineProxyReview,
  type EngineProxySnapshot,
} from "../../model/engine-proxy";
import { ConfirmDialog } from "../shell/ConfirmDialog";

export function EngineProxy({ connected }: { connected: boolean }) {
  const [supported, setSupported] = useState<boolean | null>(null);
  const [snapshot, setSnapshot] = useState<EngineProxySnapshot | null>(null);
  const [readAt, setReadAt] = useState(0);
  const [now, setNow] = useState(Date.now());
  const [mode, setMode] = useState<EngineProxyConfig["mode"]>("none");
  const [url, setUrl] = useState("");
  const [noProxy, setNoProxy] = useState("");
  const [reading, setReading] = useState(false);
  const [applying, setApplying] = useState(false);
  const [review, setReview] = useState<EngineProxyReview | null>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const live = useRef(false);
  const reader = useRef<AbortController | null>(null);
  const writer = useRef<AbortController | null>(null);
  const draftInitialized = useRef(false);
  const draftGeneration = useRef(0);
  const readGeneration = useRef(0);
  const latest = useRef<EngineProxySnapshot | null>(null);
  const fresh = !error && now - readAt >= 0 && now - readAt < 10000;

  async function read(): Promise<EngineProxySnapshot | null> {
    if (reader.current) return null;
    const abort = new AbortController();
    reader.current = abort;
    const atRead = readGeneration.current;
    setReading(true);
    let text = "",
      oversized = false;
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "readSettings" },
        (chunk, stream) => {
          if (stream !== "stdout") return;
          if (text.length + chunk.length > 1048576) {
            oversized = true;
            abort.abort();
            return;
          }
          text += chunk;
        },
        abort.signal,
      );
      if (oversized && live.current && atRead === readGeneration.current)
        setError(
          "The configuration response exceeded its limit. No setting was changed.",
        );
      if (
        abort.signal.aborted ||
        oversized ||
        !live.current ||
        atRead !== readGeneration.current
      )
        return null;
      if (!result.success)
        throw new Error(
          "The current proxy configuration could not be read. No setting was changed.",
        );
      const next = readEngineProxySnapshot(JSON.parse(text));
      const before = latest.current;
      latest.current = next;
      setSnapshot(next);
      setReadAt(writer.current ? 0 : Date.now());
      setError("");
      if (
        before &&
        (before.generation !== next.generation || before.stage !== next.stage)
      )
        window.dispatchEvent(new Event("orbwin:app-settings-changed"));
      setReview((before) =>
        before && engineProxyReviewCurrent(before, next) ? before : null,
      );
      if (!draftInitialized.current) {
        draftInitialized.current = true;
        setMode(next.desired?.mode ?? "none");
        setUrl(next.desired?.url ?? "");
        setNoProxy(next.desired?.noProxy.join("\n") ?? "");
      }
      return next;
    } catch {
      if (
        live.current &&
        !abort.signal.aborted &&
        atRead === readGeneration.current
      ) {
        setError(
          "Current proxy state is unavailable. Read it again before applying a change.",
        );
        setReview(null);
      }
      return null;
    } finally {
      if (reader.current === abort) reader.current = null;
      if (live.current) setReading(false);
    }
  }
  useEffect(() => {
    live.current = true;
    void backend
      .engineProxyCapability()
      .then((value) => {
        if (live.current) {
          setSupported(value);
          if (value) void read();
        }
      })
      .catch(() => {
        if (live.current) {
          setSupported(false);
          setError(
            "A matching qualified backend is required before proxy configuration is available.",
          );
        }
      });
    const tick = window.setInterval(() => setNow(Date.now()), 1000);
    return () => {
      live.current = false;
      reader.current?.abort();
      writer.current?.abort();
      window.clearInterval(tick);
    };
  }, []);
  useEffect(() => {
    if (!supported) return;
    const timer = window.setInterval(() => {
      if (!writer.current) void read();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [supported]);
  function changed() {
    draftInitialized.current = true;
    draftGeneration.current++;
    setReview(null);
    setMessage("");
  }
  async function reviewChange() {
    const atDraft = draftGeneration.current;
    let config: EngineProxyConfig;
    try {
      config = engineProxyConfig(mode, url, noProxy);
    } catch (cause) {
      setError(String(cause));
      return;
    }
    const current = await read();
    if (
      !current ||
      !current.running ||
      !connected ||
      !live.current ||
      atDraft !== draftGeneration.current
    )
      return;
    if (JSON.stringify(current.desired) === JSON.stringify(config)) {
      setMessage(
        "This is already the saved intent. Read its current application status.",
      );
      return;
    }
    setReview({
      uid: current.uid,
      resourceVersion: current.resourceVersion,
      generation: current.generation,
      config,
    });
  }
  async function apply() {
    const selected = review;
    if (
      !selected ||
      writer.current ||
      !connected ||
      !engineProxyReviewCurrent(selected, latest.current)
    )
      return;
    const abort = new AbortController();
    writer.current = abort;
    readGeneration.current++;
    reader.current?.abort();
    setReadAt(0);
    setReview(null);
    setApplying(true);
    setMessage("");
    setError("");
    try {
      // Private endpoint/bypass values go only in stdin to the fixed API patch.
      // CLI output is deliberately not retained in an operation log here.
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "configureEngineProxy", review: selected },
        () => {},
        abort.signal,
      );
      if (!live.current) return;
      setMessage(
        result.success
          ? "Proxy intent saved. Waiting for the backend's current status; saved does not mean ready."
          : "The change response is unconfirmed or refused. Read current state before reviewing another change; this request was not repeated.",
      );
    } catch {
      if (live.current)
        setError(
          "The change response is unconfirmed. Read current state before continuing; no request was repeated.",
        );
    } finally {
      readGeneration.current++;
      reader.current?.abort();
      if (live.current) setReadAt(0);
      if (writer.current === abort) writer.current = null;
      if (live.current) {
        setApplying(false);
        void read();
      }
    }
  }
  const ready = !applying && connected && fresh && snapshot?.stage === "ready";
  return (
    <section aria-labelledby="engine-proxy-title">
      <h2 id="engine-proxy-title">Engine proxy</h2>
      <p>
        Configure image pulls and the engine's default builder through rdd.
        Windows proxy, VPN, WSL and Docker client settings stay unchanged.
      </p>
      {supported === null && <p role="status">Checking backend support…</p>}
      {supported === false && (
        <p className="notice">
          Engine proxy configuration is unavailable in this build. A matching
          backend and completed setup are required.
        </p>
      )}
      {error && (
        <p className="notice" role="alert">
          {error}
        </p>
      )}
      {message && (
        <p className="notice" role="status">
          {message}
        </p>
      )}
      {supported && (
        <>
          <p role="status" aria-live="polite">
            {ready
              ? "Ready for the current saved policy."
              : !snapshot || !fresh
                ? "Current proxy state is unconfirmed."
                : snapshot.stage === "unmanaged"
                  ? "No OrbWin proxy policy has been configured."
                  : snapshot.stage === "unavailable"
                    ? "The backend could not apply the current policy. Inspect engine diagnostics before changing it."
                    : "The backend is applying or verifying the saved policy."}
          </p>
          {snapshot && (
            <dl className="properties">
              <dt>Saved mode</dt>
              <dd>{snapshot.desired?.mode ?? "Unmanaged"}</dd>
              {snapshot.desired?.mode === "url" && (
                <>
                  <dt>Saved endpoint</dt>
                  <dd>
                    <code>{snapshot.desired.url}</code>
                  </dd>
                </>
              )}
            </dl>
          )}
          <div className="action-row">
            <button
              className="button"
              disabled={reading}
              onClick={() => void read()}
            >
              {reading ? "Reading…" : "Read current state"}
            </button>
            {reading && (
              <button
                className="button"
                onClick={() => reader.current?.abort()}
              >
                Cancel read
              </button>
            )}
          </div>
          <label className="form-field">
            Mode
            <select
              value={mode}
              disabled={applying}
              onChange={(event) => {
                changed();
                setMode(event.target.value as EngineProxyConfig["mode"]);
              }}
            >
              <option value="none">None — direct connections</option>
              <option value="url">URL — explicit proxy</option>
              <option value="auto">Auto — Windows user configuration</option>
            </select>
          </label>
          {mode === "url" && (
            <label className="form-field">
              Proxy URL
              <input
                type="text"
                autoComplete="off"
                spellCheck={false}
                value={url}
                disabled={applying}
                placeholder="http://proxy.example:8080"
                onChange={(event) => {
                  changed();
                  setUrl(event.target.value);
                }}
              />
            </label>
          )}
          <label className="form-field">
            Bypass hosts, addresses or networks
            <textarea
              rows={3}
              value={noProxy}
              disabled={applying}
              autoComplete="off"
              spellCheck={false}
              placeholder="One entry per line"
              onChange={(event) => {
                changed();
                setNoProxy(event.target.value);
              }}
            />
          </label>
          <p className="settings-hint">
            Credential-bearing URLs are unsupported. Auto may fetch and evaluate
            Windows PAC/WPAD configuration for each destination; authentication
            is not supplied automatically. None uses direct connections for new
            requests. Existing proxy streams can drain under their earlier
            policy.
          </p>
          <button
            className="button"
            disabled={applying || reading || !connected || !snapshot}
            onClick={() => void reviewChange()}
          >
            {applying ? "Submitting…" : "Review proxy change…"}
          </button>
          {applying && (
            <button className="button" onClick={() => writer.current?.abort()}>
              Cancel waiting
            </button>
          )}
          {review && (
            <ConfirmDialog
              title="Apply engine proxy policy?"
              destructive
              confirmLabel="Apply proxy policy"
              confirmDisabled={
                !connected || !engineProxyReviewCurrent(review, snapshot)
              }
              onCancel={() => setReview(null)}
              onConfirm={() => void apply()}
            >
              <p>
                This can restart the owned Docker daemon and interrupt Docker
                clients and workloads. Data is retained. Cancellation after
                submission stops waiting; it does not roll back the saved policy
                or a daemon restart.
              </p>
              <dl className="properties">
                <dt>Mode</dt>
                <dd>{review.config.mode}</dd>
                {review.config.mode === "url" && (
                  <>
                    <dt>Endpoint</dt>
                    <dd>
                      <code>{review.config.url}</code>
                    </dd>
                  </>
                )}
                <dt>Bypass</dt>
                <dd>
                  {review.config.noProxy.join(", ") || "No explicit entries"}
                </dd>
              </dl>
              {review.config.mode === "auto" && (
                <p>
                  Auto authorizes the bounded PAC/WPAD requests described above.
                  Failure is reported; it does not silently switch to direct
                  networking.
                </p>
              )}
            </ConfirmDialog>
          )}
        </>
      )}
    </section>
  );
}
