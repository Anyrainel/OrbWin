import { createContext, useContext, useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { formatSize } from "../settings/storage-model";
import {
  parseVolumeSize,
  type VolumeSize as Measurement,
} from "./volume-size-model";

export const VolumeSizeObservation = createContext<
  (value: Measurement) => void
>(() => {});

export function VolumeSize({
  name,
  createdAt,
}: {
  name: string;
  createdAt: string;
}) {
  const pending = useRef<AbortController | null>(null);
  const observe = useContext(VolumeSizeObservation);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [measurement, setMeasurement] = useState<Measurement | null>(null);
  useEffect(
    () => () => {
      pending.current?.abort();
      pending.current = null;
    },
    [],
  );
  async function read() {
    if (pending.current) return;
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setMessage("");
    let output = "",
      diagnostic = "",
      overflow = false;
    try {
      const outcome = await backend.run(
        crypto.randomUUID(),
        { kind: "volumeSize", target: name, createdAt },
        (chunk, stream) => {
          if (stream === "stderr") {
            diagnostic = (diagnostic + chunk).slice(0, 8192);
            return;
          }
          if (output.length + chunk.length > 8192) overflow = true;
          else output += chunk;
        },
        abort.signal,
      );
      if (abort.signal.aborted) return;
      if (!outcome.success)
        throw new Error(`${outcome.message} ${diagnostic.trim()}`);
      if (overflow) throw new Error("Volume size output exceeded its limit.");
      const value = parseVolumeSize(output, name, createdAt);
      setMeasurement(value);
      observe(value);
    } catch (error) {
      if (!abort.signal.aborted) setMessage(String(error));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
        if (abort.signal.aborted)
          setMessage("Measurement cancelled; earlier results retained.");
      }
    }
  }
  return (
    <section className="info-section" aria-label="Volume size">
      <h3>Engine-reported size</h3>
      <button className="button" disabled={busy} onClick={() => void read()}>
        {busy ? "Measuring volume…" : "Refresh volume size"}
      </button>
      {busy && (
        <button
          className="button"
          onClick={() => {
            pending.current?.abort();
            setMessage("Cancelling measurement…");
          }}
        >
          Cancel measurement
        </button>
      )}
      <p role="status">
        {message ||
          (busy
            ? "Reading volume accounting…"
            : measurement
              ? `Last measured ${new Date(measurement.measuredAt).toLocaleString()}.`
              : "Size has not been measured.")}
        {message && measurement
          ? ` Last successful measurement: ${new Date(measurement.measuredAt).toLocaleString()}.`
          : ""}
      </p>
      {measurement && (
        <dl className="detail-properties">
          <div>
            <dt>Size</dt>
            <dd>
              {formatSize(measurement.bytes)} (
              {measurement.bytes.toLocaleString()} bytes)
            </dd>
          </div>
          <div>
            <dt>Container references</dt>
            <dd>{measurement.references}</dd>
          </div>
        </dl>
      )}
      <p>
        Size refresh supports ordinary local volumes without driver options.
        Docker scans volume usage across the engine. Writers can change data
        during measurement; this is not a consistent backup size or Windows disk
        allocation. References include stopped containers. Cancel closes this
        client request; work already started in the engine may continue.
      </p>
    </section>
  );
}
