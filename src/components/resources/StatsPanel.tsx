import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { append } from "./model";
import { containDialogTab } from "../shell/dialog-focus";
import { ActivitySampler } from "./activity-sampler";
import {
  formatBytes,
  parseStats,
  sampleLimit,
  samplingIntervals,
  type StatsSample,
} from "./stats-model";

function Sparkline({
  label,
  values,
  percent = false,
}: {
  label: string;
  values: number[];
  percent?: boolean;
}) {
  const format = percent
    ? (value: number) => `${value.toFixed(1)}%`
    : formatBytes;
  const latest = values.at(-1);
  const peak = Math.max(0, ...values);
  const top = Math.max(percent ? 100 : 1, peak);
  const points = values
    .map(
      (value, index) =>
        `${((sampleLimit - values.length + index) / (sampleLimit - 1)) * 120},${30 - (value / top) * 28}`,
    )
    .join(" ");
  return (
    <figure className="sparkline">
      <figcaption>
        <span>{label}</span>
        <strong>{latest === undefined ? "—" : format(latest)}</strong>
      </figcaption>
      <svg
        viewBox="0 0 120 32"
        preserveAspectRatio="none"
        role="img"
        aria-label={
          values.length
            ? `${label}: latest ${format(latest!)}, peak ${format(peak)} over ${values.length} samples`
            : `${label}: no samples yet`
        }
      >
        <line x1="0" x2="120" y1="30" y2="30" className="sparkline-base" />
        {points && <polyline points={points} />}
      </svg>
    </figure>
  );
}

// A target change gets a fresh polling owner and bounded, view-local history.
export function StatsPanel({ target }: { target?: string }) {
  return <StatsView key={target ?? "engine"} target={target} />;
}
function StatsView({ target }: { target?: string }) {
  const [samples, setSamples] = useState<StatsSample[]>([]);
  const [message, setMessage] = useState("");
  const [pending, setPending] = useState(false);
  const [live, setLive] = useState(true);
  const [interval, setIntervalMs] = useState(5000);
  const [sampledAt, setSampledAt] = useState("");
  const sampler = useRef<ActivitySampler<StatsSample> | null>(null);
  useEffect(() => {
    const owner = new ActivitySampler<StatsSample>(
      async (signal) => {
        let output = "";
        let overflow = false;
        const outcome = await backend.run(
          crypto.randomUUID(),
          target ? { kind: "stats", target } : { kind: "engineStats" },
          (chunk, stream) => {
            if (stream === "stderr") return;
            if (output.length + chunk.length > 2 * 1024 * 1024) {
              overflow = true;
            } else output += chunk;
          },
          signal,
        );
        if (overflow)
          throw new Error(
            "Stats exceeded the view's output limit. Narrow the selection.",
          );
        signal.throwIfAborted();
        if (!outcome.success) throw new Error(outcome.message);
        return parseStats(output, !target, target);
      },
      (event) => {
        if (event.kind === "reading") {
          setPending(true);
          setMessage("");
        } else if (event.kind === "cancelling") setMessage("Cancelling read…");
        else if (event.kind === "cancelled") {
          setPending(false);
          setMessage("Read cancelled; previous samples retained.");
        } else if (event.kind === "settled") setPending(false);
        else if (event.kind === "error") setMessage(event.message);
        else if (event.kind === "sample") {
          setSamples((history) => append(history, event.sample, sampleLimit));
          setSampledAt(new Date().toLocaleTimeString());
          setMessage("");
        }
      },
      {
        defer: (callback) => {
          const timer = window.setTimeout(callback, 0);
          return () => window.clearTimeout(timer);
        },
        repeat: (callback, milliseconds) => {
          const timer = window.setInterval(callback, milliseconds);
          return () => window.clearInterval(timer);
        },
        visible: () => document.visibilityState === "visible",
      },
    );
    sampler.current = owner;
    return () => {
      owner.dispose();
      if (sampler.current === owner) sampler.current = null;
    };
  }, []);
  useEffect(() => {
    sampler.current?.configure(live, interval);
  }, [live, interval]);
  return (
    <section
      className="stats-panel"
      aria-label={target ? "Container activity" : "Engine container activity"}
    >
      <div className="action-row">
        <label>
          <input
            type="checkbox"
            checked={live}
            onChange={(event) => setLive(event.target.checked)}
          />{" "}
          Live sampling
        </label>
        <label>
          Interval{" "}
          <select
            value={interval}
            onChange={(event) => setIntervalMs(Number(event.target.value))}
          >
            {samplingIntervals.map((value) => (
              <option key={value} value={value}>
                {value / 1000} seconds
              </option>
            ))}
          </select>
        </label>
        <button
          className="button"
          disabled={pending}
          onClick={() => void sampler.current?.read()}
        >
          Read current stats
        </button>
        {pending && (
          <button
            className="button"
            onClick={() => {
              setLive(false);
              sampler.current?.cancel();
            }}
          >
            Cancel read
          </button>
        )}
      </div>
      <p role="status">
        {message
          ? `${message} ${sampledAt ? `Charts retain the last successful sample at ${sampledAt}.` : ""}`
          : pending
            ? "Reading stats…"
            : samples.length
              ? `Last sample ${sampledAt}. ${samples.length} of ${sampleLimit} samples retained in this view only.`
              : "No samples yet."}
      </p>
      {!target && (
        <p>
          Totals across {samples.at(-1)?.containers ?? "—"} running containers,
          including Kubernetes. CPU can exceed 100% across cores. These totals
          exclude the engine VM's own resource use.
        </p>
      )}
      <div className="sparklines">
        <Sparkline
          label="CPU"
          values={samples.map((sample) => sample.cpu)}
          percent
        />
        <Sparkline
          label="Memory usage"
          values={samples.map((sample) => sample.memory)}
        />
        <Sparkline
          label="Network received"
          values={samples.map((sample) => sample.networkIn)}
        />
        <Sparkline
          label="Network sent"
          values={samples.map((sample) => sample.networkOut)}
        />
        <Sparkline
          label="Block read"
          values={samples.map((sample) => sample.blockRead)}
        />
        <Sparkline
          label="Block written"
          values={samples.map((sample) => sample.blockWrite)}
        />
      </div>
      <p>
        Memory is current usage. I/O shows rounded cumulative counters, not
        rates; container restarts or membership changes can lower totals.
        Sampling pauses while this window is hidden. Closing this view discards
        history.
      </p>
    </section>
  );
}

export function EngineActivity() {
  const [open, setOpen] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const dismiss = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open || !dialog.current) return;
    const element = dialog.current;
    element.showModal();
    dismiss.current?.focus();
    return () => {
      element.close();
      trigger.current?.focus();
    };
  }, [open]);
  return (
    <div>
      <button
        className="button"
        ref={trigger}
        aria-expanded={open}
        aria-controls="engine-activity"
        onClick={() => setOpen(true)}
      >
        Engine activity
      </button>
      {open && (
        <dialog
          ref={dialog}
          id="engine-activity"
          className="operation-dialog"
          aria-labelledby="engine-activity-title"
          onKeyDown={containDialogTab}
          onCancel={(event) => {
            event.preventDefault();
            setOpen(false);
          }}
        >
          <h2 id="engine-activity-title">Engine container activity</h2>
          <button
            ref={dismiss}
            className="button"
            onClick={() => setOpen(false)}
          >
            Close activity
          </button>
          <StatsPanel />
        </dialog>
      )}
    </div>
  );
}
