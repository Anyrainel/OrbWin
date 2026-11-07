import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { parseMigrationDestination } from "../../model/migration-destination";

export function MigrationDestinationReview({
  source,
  sourceVolume,
  destination,
  target,
}: {
  source: string;
  sourceVolume: string;
  destination: string;
  target: string;
}) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const pending = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      pending.current?.abort();
      pending.current = null;
    },
    [],
  );
  async function check() {
    if (busy || !destination) return;
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setMessage("");
    let output = "",
      diagnostic = "",
      overflow = false;
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        {
          kind: "reviewMigrationDestination",
          source,
          sourceVolume,
          destination,
        },
        (chunk, stream) => {
          if (stream === "stderr")
            diagnostic = (diagnostic + chunk).slice(-8192);
          else if (output.length + chunk.length > 65536) {
            overflow = true;
            abort.abort();
          } else output += chunk;
        },
        abort.signal,
      );
      if (pending.current !== abort) return;
      if (abort.signal.aborted || overflow)
        throw new Error(
          "Destination check cancelled or exceeded its response budget. No data was changed.",
        );
      if (!result.success) throw new Error(diagnostic || result.message);
      const checked = parseMigrationDestination(output, {
        source,
        sourceVolume,
        destination,
        target,
      });
      setMessage(
        `Restore ${checked.restoreTransaction} matches ${checked.exportTransactions.length} source export acknowledgement(s). Destination metadata matched at ${checked.observedAt}. This does not verify current writable contents or reserve the volume.`,
      );
    } catch (reason) {
      if (pending.current === abort) setMessage(String(reason));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  return (
    <div className="migration-destination-check">
      <button
        className="button compact"
        disabled={busy || !destination}
        onClick={() => void check()}
      >
        {busy
          ? "Checking recorded restore…"
          : "Check restore receipt and destination"}
      </button>
      {busy && (
        <button
          className="button compact"
          onClick={() => pending.current?.abort()}
        >
          Cancel check
        </button>
      )}
      {message && <p role="status">{message}</p>}
    </div>
  );
}
