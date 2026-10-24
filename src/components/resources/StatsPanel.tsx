import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { append, percent } from "./model";

type Sample = { cpu?: number; memory?: number; raw: Record<string, unknown> };
const limit = 60;
const interval = 3000;

function Sparkline({
  label,
  values,
}: {
  label: string;
  values: (number | undefined)[];
}) {
  const known = values.filter((value): value is number => value !== undefined);
  const latest = known.at(-1);
  // CPU can exceed 100% on multiple cores; scale to the larger of both.
  const top = Math.max(100, ...known);
  const points = values
    .map((value, index) =>
      value === undefined
        ? null
        : // Right-aligned so the newest sample is always at the edge.
          `${((limit - values.length + index) / (limit - 1)) * 120},${30 - (value / top) * 28}`,
    )
    .filter(Boolean)
    .join(" ");
  return (
    <figure className="sparkline">
      <figcaption>
        <span>{label}</span>
        <strong>{latest === undefined ? "—" : `${latest.toFixed(1)}%`}</strong>
      </figcaption>
      <svg
        viewBox="0 0 120 32"
        preserveAspectRatio="none"
        role="img"
        aria-label={
          known.length
            ? `${label}: latest ${latest!.toFixed(1)}%, peak ${Math.max(...known).toFixed(1)}% over ${known.length} samples`
            : `${label}: no samples yet`
        }
      >
        <line x1="0" x2="120" y1="30" y2="30" className="sparkline-base" />
        {points && <polyline points={points} />}
      </svg>
    </figure>
  );
}

// Polls `docker stats --no-stream` while visible. History lives only in this
// component (bounded to `limit` samples) and resets when the target changes.
export function StatsPanel({ target }: { target: string }) {
  const [samples, setSamples] = useState<Sample[]>([]);
  const [message, setMessage] = useState("");
  const [pending, setPending] = useState(false);
  const [live, setLive] = useState(true);
  const active = useRef(false);
  const controller = useRef<AbortController | null>(null);
  async function read() {
    if (active.current) return;
    active.current = true;
    setPending(true);
    const abort = new AbortController();
    controller.current = abort;
    let output = "";
    try {
      const outcome = await backend.run(
        crypto.randomUUID(),
        { kind: "stats", target },
        (chunk) => {
          output += chunk;
        },
        abort.signal,
      );
      if (abort.signal.aborted) return;
      if (!outcome.success) return setMessage(outcome.message);
      const raw = JSON.parse(output.trim().split("\n").at(-1) ?? "");
      setSamples((history) =>
        append(
          history,
          { cpu: percent(raw.CPUPerc), memory: percent(raw.MemPerc), raw },
          limit,
        ),
      );
      setMessage("");
    } catch (error) {
      if (!abort.signal.aborted)
        setMessage(
          error instanceof SyntaxError
            ? "The engine returned an unreadable stats sample."
            : String(error),
        );
    } finally {
      active.current = false;
      setPending(false);
    }
  }
  useEffect(() => {
    // Deferred so a development double-mount cancels before reading.
    const timer = window.setTimeout(() => void read(), 0);
    return () => {
      window.clearTimeout(timer);
      controller.current?.abort();
    };
  }, [target]);
  useEffect(() => {
    if (!live) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") void read();
    }, interval);
    return () => window.clearInterval(timer);
  }, [target, live]);
  const latest = samples.at(-1)?.raw;
  return (
    <section className="stats-panel">
      <div className="action-row">
        <label>
          <input
            type="checkbox"
            checked={live}
            onChange={(e) => setLive(e.target.checked)}
          />{" "}
          Refresh every 3 seconds
        </label>
        <button
          className="button"
          disabled={pending}
          onClick={() => void read()}
        >
          Read current stats
        </button>
      </div>
      <p role="status">
        {message ||
          (samples.length
            ? `${samples.length} of ${limit} samples retained in this view only.`
            : pending
              ? "Reading stats…"
              : "No samples yet.")}
      </p>
      <div className="sparklines">
        <Sparkline label="CPU" values={samples.map((s) => s.cpu)} />
        <Sparkline label="Memory" values={samples.map((s) => s.memory)} />
      </div>
      {latest && (
        <dl className="detail-properties">
          {(
            [
              ["Memory usage", latest.MemUsage],
              ["Network I/O", latest.NetIO],
              ["Block I/O", latest.BlockIO],
              ["Processes", latest.PIDs],
            ] as const
          ).map(([label, value]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{String(value ?? "—")}</dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}
