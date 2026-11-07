// Development-only visual/keyboard fixture. All actions stay in this component;
// no backend API is imported or called and no system trust can change.
import { useState } from "react";
import { createRoot } from "react-dom/client";
import type { LocalServiceState, localServices } from "../../../src/backend";
import { WslTrustControls } from "../../../src/components/settings/WslTrustControls";
import { WslTrustReceipts } from "../../../src/components/settings/WslTrustReceipts";
import "../../../src/styles/shell.css";
import "../../../src/styles.css";

const id = "00000000-0000-4000-8000-000000000002";
const root = "b".repeat(64);
const initial: LocalServiceState = {
  instance: "a".repeat(32),
  sequence: 1,
  revision: 7,
  settings: {
    enabled: false,
    ports: { http: 18080, https: 18443 },
    rootSha256: root,
  },
  phase: "disabled",
  updatedAt: "2026-10-25T00:00:00Z",
  wslDistributionsRead: true,
  wslDistributions: [{ id, name: "Ubuntu 24.04" }],
  wslTrust: {
    distributionId: id,
    rootSha256: root,
    ownerState: "unmanaged",
    ownerSequence: 0,
    observed: true,
    managed: false,
    guest: {
      distributionId: id,
      rootSha256: root,
      machineId: "c".repeat(32),
      osId: "ubuntu",
      osVersion: "24.04",
      observed: true,
      filePresent: false,
      bundlePresent: false,
      managed: false,
      state: "absent",
      sequence: 0,
      updaterWarning: false,
    },
  },
};
function Fixture() {
  const [mode, setMode] = useState("absent");
  const [last, setLast] = useState("");
  const state = structuredClone(initial);
  const status = state.wslTrust!;
  const guest = status.guest!;
  if (["installed", "pending", "warning", "preexisting"].includes(mode)) {
    guest.filePresent = guest.bundlePresent = true;
  }
  if (["installed", "pending", "warning"].includes(mode)) {
    status.ownerState = mode === "pending" ? "install-intent" : "installed";
    status.ownerSequence = 2;
    status.managed = guest.managed = true;
    guest.state = "installed";
    guest.sequence = 2;
  }
  if (mode === "preexisting") {
    guest.state = "present-unmanaged";
  }
  if (mode === "warning") guest.updaterWarning = true;
  if (mode === "unknown") {
    status.ownerState = "install-intent";
    status.ownerSequence = 1;
    status.observed = false;
    delete status.guest;
  }
  if (mode === "empty") state.wslDistributions = [];
  if (mode === "rotation") state.phase = "rotation-pending";
  if (mode === "unsafe-sequence") status.ownerSequence = 2 ** 53;
  const receiptMode = mode.startsWith("receipt-");
  if (receiptMode) {
    state.wslTrustReceiptsRead = true;
    state.wslTrustReceipts = [
      {
        distributionId: id,
        machineId: "c".repeat(32),
        rootSha256: root,
        state:
          mode === "receipt-pending"
            ? "registration-retiring"
            : mode === "receipt-retired"
              ? "registration-retired"
              : "installed",
        ownerSequence: mode === "receipt-unsafe-sequence" ? 2 ** 53 : 2,
        guestSequence: 2,
        registrationState:
          mode === "receipt-present"
            ? "present"
            : mode === "receipt-unknown"
              ? "unknown"
              : "absent",
        registrationError:
          mode === "receipt-unknown"
            ? "Controlled registry read failure."
            : undefined,
        retiredAt:
          mode === "receipt-retired" ? "2026-10-25T09:00:00Z" : undefined,
      },
    ];
    if (mode === "receipt-read-error") {
      state.wslTrustReceipts = [];
      state.wslTrustReceiptsError = "Controlled public-receipt read failure.";
    }
    if (mode === "receipt-empty") state.wslTrustReceipts = [];
  }
  async function run(request: Parameters<typeof localServices>[0]) {
    setLast(JSON.stringify(request, null, 2));
  }
  return (
    <main style={{ maxWidth: 820, margin: "24px auto", padding: "0 20px" }}>
      <h1>WSL trust interaction fixture</h1>
      <p className="notice">
        Fixture only. No backend connection or system trust changes.
      </p>
      <label>
        Fixture state{" "}
        <select
          value={mode}
          onChange={(event) => {
            setMode(event.target.value);
            setLast("");
          }}
        >
          {[
            "absent",
            "installed",
            "pending",
            "warning",
            "preexisting",
            "unknown",
            "empty",
            "rotation",
            "unsafe-sequence",
            "receipt-absent",
            "receipt-present",
            "receipt-unknown",
            "receipt-pending",
            "receipt-retired",
            "receipt-read-error",
            "receipt-empty",
            "receipt-unsafe-sequence",
            "receipt-busy",
          ].map((value) => (
            <option key={value}>{value}</option>
          ))}
        </select>
      </label>
      {receiptMode ? (
        <WslTrustReceipts
          state={state}
          busy={mode === "receipt-busy"}
          run={run}
        />
      ) : (
        <WslTrustControls state={state} busy={false} run={run} />
      )}
      <h2>Last fixture request</h2>
      <pre
        aria-live="polite"
        style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}
      >
        {last || "No action submitted."}
      </pre>
    </main>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
