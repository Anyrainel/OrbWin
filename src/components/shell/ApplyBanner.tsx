import { RefreshCw } from "lucide-react";
import type { AppSettingsRead } from "../settings/useAppSettings";
import { applyState } from "../settings/app-settings-model";

// Shown while rdd reports that its reconcile chain has not settled on the
// current App generation (Settled condition). This is the backend's own
// pending-change signal; OrbWin does not infer per-setting restart needs.
export function ApplyBanner({
  read,
  onReload,
}: {
  read: AppSettingsRead;
  onReload: () => void;
}) {
  if (read.kind !== "ready") return null;
  const state = applyState(read.settings);
  if (state.kind !== "applying") return null;
  return (
    <div className="apply-banner" role="status">
      <span>
        <strong>Backend is applying configuration.</strong>{" "}
        {state.reason ? `Reason: ${state.reason}. ` : ""}
        {state.message ?? ""} Workloads may be unavailable until it settles.
      </span>
      <button className="button compact" onClick={onReload}>
        <RefreshCw size={13} aria-hidden="true" /> Re-read
      </button>
    </div>
  );
}
