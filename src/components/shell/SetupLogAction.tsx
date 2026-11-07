import { useEffect, useId, useRef, useState } from "react";
import * as backend from "../../backend";
import { setupLogErrorMessage } from "../../model/setup-log-action";

export function SetupLogAction() {
  const [opening, setOpening] = useState(false);
  const [error, setError] = useState("");
  const active = useRef(false);
  const mounted = useRef(false);
  const messageId = useId();
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  async function open() {
    if (active.current) return;
    active.current = true;
    setOpening(true);
    setError("");
    try {
      await backend.openSetupLogs();
    } catch (reason) {
      if (mounted.current) setError(setupLogErrorMessage(reason));
    } finally {
      active.current = false;
      if (mounted.current) setOpening(false);
    }
  }
  return (
    <div>
      <button
        className="button"
        disabled={opening}
        aria-busy={opening}
        aria-describedby={error ? messageId : undefined}
        onClick={() => void open()}
      >
        {opening ? "Opening setup logs…" : "Open setup logs"}
      </button>
      {opening && <p role="status">Opening setup logs in File Explorer.</p>}
      {error && (
        <p id={messageId} role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
