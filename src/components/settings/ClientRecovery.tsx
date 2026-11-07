import { useEffect, useId, useRef, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import * as backend from "../../backend";
import {
  reviewClientBootstrap,
  type ClientBootstrapSelection,
} from "../../client-bootstrap";

export function ClientRecovery({
  disabled,
  onBusy,
}: {
  disabled: boolean;
  onBusy: (busy: boolean) => void;
}) {
  const [action, setAction] = useState<"recover" | ClientBootstrapSelection>("recover");
  const actionHint = useId();
  const [review, setReview] = useState<backend.ClientRecoveryReview | null>(
    null,
  );
  const [phase, setPhase] = useState<
    "idle" | "reviewing" | "applying" | "closing"
  >("idle");
  const [message, setMessage] = useState("");
  const [failed, setFailed] = useState(false);
  const reviewButton = useRef<HTMLButtonElement>(null);
  const applyButton = useRef<HTMLButtonElement>(null);
  const focusReview = useRef(false);
  const mounted = useRef(false);
  const retained = useRef<backend.ClientRecoveryReview | null>(null);
  const active = useRef(false);
  const abandon = useRef(false);
  const notify = useRef(onBusy);
  notify.current = onBusy;

  useEffect(() => {
    if (review) applyButton.current?.focus();
    else if (phase === "idle" && focusReview.current) {
      focusReview.current = false;
      reviewButton.current?.focus();
    }
  }, [review, phase]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      const owner = retained.current;
      retained.current = null;
      if (owner) void backend.cancelClientRecovery(owner.epoch).catch(() => {});
    };
  }, []);
  useEffect(() => {
    if (!review) return;
    const timer = window.setTimeout(() => {
      if (active.current || retained.current?.epoch !== review.epoch) return;
      active.current = true;
      retained.current = null;
      setReview(null);
      setPhase("closing");
      setFailed(false);
      setMessage("");
      // Expiry still owns the native controller until checked release finishes.
      // Keep competing settings actions disabled and report a release failure.
      void backend
        .cancelClientRecovery(review.epoch)
        .then(
          () => {
            if (mounted.current)
              setMessage(
                "Settings review expired. Review again before applying changes.",
              );
          },
          (error: unknown) => {
            if (mounted.current) {
              setMessage(String(error));
              setFailed(true);
            }
          },
        )
        .finally(() => {
          active.current = false;
          if (mounted.current) {
            focusReview.current = true;
            setPhase("idle");
            notify.current(false);
          }
        });
    }, review.expiresInSeconds * 1000);
    return () => window.clearTimeout(timer);
  }, [review]);

  async function inspect() {
    if (active.current || retained.current) return;
    active.current = true;
    abandon.current = false;
    onBusy(true);
    setPhase("reviewing");
    setMessage("");
    setFailed(false);
    try {
      const result = action === "recover"
        ? await backend.reviewClientRecovery()
        : await reviewClientBootstrap(action);
      if (!result) {
        if (mounted.current)
          setMessage("No pending work found for the selected settings action.");
        return;
      }
      if (!mounted.current || abandon.current) {
        await backend.cancelClientRecovery(result.epoch);
        if (mounted.current)
          setMessage("Review closed. Pending recovery records were retained.");
        return;
      }
      retained.current = result;
      setReview(result);
    } catch (error) {
      if (mounted.current) {
        setMessage(String(error));
        setFailed(true);
      }
    } finally {
      active.current = false;
      if (mounted.current) {
        setPhase("idle");
        if (!retained.current) onBusy(false);
      }
    }
  }
  async function finish(apply: boolean) {
    const owner = retained.current;
    if (active.current || !owner) return;
    active.current = true;
    retained.current = null;
    setReview(null);
    setPhase(apply ? "applying" : "closing");
    setMessage("");
    setFailed(false);
    try {
      if (apply) await backend.restoreClientRecovery(owner.epoch);
      else await backend.cancelClientRecovery(owner.epoch);
      if (mounted.current)
        setMessage(
          apply
            ? action === "recover"
              ? "Private Docker settings recovery completed. Refresh saved credentials before signing in again."
              : action === "empty"
                ? "Empty private Docker settings initialized."
                : "Private Docker settings import completed. Historical source files remain unchanged."
            : "Review closed. Pending recovery records were retained.",
        );
    } catch (error) {
      if (mounted.current) {
        setMessage(String(error));
        setFailed(true);
      }
    } finally {
      active.current = false;
      if (mounted.current) {
        focusReview.current = true;
        setPhase("idle");
        onBusy(false);
      }
    }
  }
  return (
    <>
      <h3>Private Docker settings</h3>
      <p className="settings-hint">
        If initial setup, a sign-in, or a settings change was interrupted,
        review the preserved private Docker settings before applying recovery.
        Recovery does not repeat a login or undo changes already made in Windows
        Credential Manager. Running workloads remain independent.
      </p>
      <label className="form-field">
        Settings action
        <select
          value={action}
          aria-describedby={actionHint}
          disabled={!isTauri() || disabled || phase !== "idle" || review !== null}
          onChange={(event) => {
            const next = event.target.value;
            if (next === "recover" || next === "empty" || next === "import-current-source" || next === "resume-import") {
              setAction(next);
              setMessage("");
              setFailed(false);
            }
          }}
        >
          <option value="recover">Recover an interrupted settings write</option>
          <option value="empty">Initialize empty private settings</option>
          <option value="import-current-source">Review previous OrbWin settings for import</option>
          <option value="resume-import">Continue an interrupted import</option>
        </select>
      </label>
      <p id={actionHint} className="settings-hint">
        {action === "recover"
          ? "Review preserved settings before applying recovery."
          : action === "empty"
            ? "Empty setup requires no historical OrbWin client directory. Existing private settings are not merged or replaced."
            : action === "import-current-source"
              ? "Review only the previous OrbWin application's settings. Global Docker settings and other engines are not imported. Any uncommitted import preparation that can be discarded will be identified in the review before you apply."
              : "Continue using the protected source captured by the interrupted import. If no complete source was captured, choose a new import review; the old source cannot be reconstructed from partial records."}
      </p>
      {!review && (
        <button
          ref={reviewButton}
          className="button"
          disabled={!isTauri() || disabled || phase !== "idle"}
          onClick={() => void inspect()}
        >
          Review selected action
        </button>
      )}
      {review && (
        <div role="region" aria-label="Private Docker settings review">
          <p>{review.summary}</p>
          <p className="settings-hint">
            This review expires after five minutes.
          </p>
          <div className="action-row">
            <button
              ref={applyButton}
              className="button primary"
              onClick={() => void finish(true)}
            >
              Apply reviewed settings
            </button>
            <button className="button" onClick={() => void finish(false)}>
              Cancel
            </button>
          </div>
        </div>
      )}
      {phase !== "idle" && (
        <p role="status">
          {phase === "reviewing"
            ? "Reviewing private settings…"
            : phase === "applying"
              ? "Applying reviewed settings. This continues if you leave this page…"
              : "Closing review…"}
        </p>
      )}
      {phase === "reviewing" && (
        <button
          className="button"
          onClick={() => {
            abandon.current = true;
            setPhase("closing");
          }}
        >
          Cancel review
        </button>
      )}
      {message && <p role={failed ? "alert" : "status"}>{message}</p>}
    </>
  );
}
