import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import {
  lanClientNetworks,
  lanOptionsCurrent,
  lanRetainChoices,
  lanPublicationKey,
  lanParsePending,
  lanObservePending,
  lanReconcilePending,
  type LANPendingChange,
  lanIngressFresh,
  lanReason,
  lanReviewCurrent,
  lanReviewOperation,
  type LANAccessStatus,
  type LANOptions,
  type LANReview,
} from "../../model/lan-access";
import { ConfirmDialog } from "../shell/ConfirmDialog";

const publicationKey = lanPublicationKey;
const pendingStorageKey = "orbwin.lan.pending-reviews.v1";
const phaseText: Record<LANAccessStatus["phase"], string> = {
  "awaiting-admin": "Waiting for administrator approval",
  "awaiting-confirmation": "Waiting for scope confirmation",
  staging: "Preparing LAN access",
  starting: "Starting access",
  ready: "Ready",
  closing: "Closing access",
  closed: "Access closed",
  cancelled: "Cancelled",
  "cleanup-pending": "Cleanup needs attention",
  removed: "Firewall rule removed",
  failed: "LAN setup failed",
};
const firewallText: Record<LANAccessStatus["firewall"], string> = {
  "not-dispatched": "No rule change dispatched",
  created: "Rule created",
  removed: "Rule removed",
  "create-unknown": "Rule creation unconfirmed",
  "remove-unknown": "Rule removal unconfirmed",
};

export function LanAccess({
  state,
  connected,
  busy,
  receive,
  refresh,
}: {
  state: backend.LocalServiceState;
  connected: boolean;
  busy: boolean;
  receive: (response: {
    state?: backend.LocalServiceState;
    error?: string;
  }) => void;
  refresh: () => Promise<void>;
}) {
  const [options, setOptions] = useState<LANOptions | null>(null);
  const [publication, setPublication] = useState("");
  const [network, setNetwork] = useState("");
  const [address, setAddress] = useState("");
  const [clients, setClients] = useState("");
  const [review, setReview] = useState<{
    value: LANReview;
    revision: number;
    runtimeInstance: string;
  } | null>(null);
  const [active, setActive] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [storageError, setStorageError] = useState("");
  const [pending, setPending] = useState<LANPendingChange[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const [readDismissed, setReadDismissed] = useState(false);
  const [now, setNow] = useState(Date.now());
  const live = useRef(true);
  const inFlight = useRef(new Set<string>());
  const readGeneration = useRef(0);
  const pendingRef = useRef<LANPendingChange[]>([]);
  const storageReady = useRef(false);
  const revision = useRef(state.revision);
  const instance = useRef(state.instance);
  revision.current = state.revision;
  instance.current = state.instance;
  useEffect(() => {
    live.current = true;
    try {
      const retained = lanParsePending(
        window.localStorage.getItem(pendingStorageKey),
      );
      pendingRef.current = retained;
      setPending(retained);
      // Verify persistence works before offering a new activation.
      window.localStorage.setItem(pendingStorageKey, JSON.stringify(retained));
      storageReady.current = true;
    } catch {
      setStorageError(
        "LAN recovery storage is unavailable or invalid. New activation is blocked; retained operations can still be inspected, cancelled, or removed.",
      );
    }
    setLoaded(true);
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => {
      live.current = false;
      readGeneration.current++;
      window.clearInterval(timer);
    };
  }, []);
  function retainPending(records: LANPendingChange[]) {
    // Unknown records are never evicted to make space for another request.
    pendingRef.current = records;
    setPending(records);
    if (!storageReady.current) return false;
    try {
      const raw = JSON.stringify(records);
      lanParsePending(raw);
      window.localStorage.setItem(pendingStorageKey, raw);
      return true;
    } catch {
      storageReady.current = false;
      setStorageError(
        "LAN recovery storage is unavailable or full. New activation is blocked; recovery controls remain available.",
      );
      return false;
    }
  }
  useEffect(() => {
    setReview(null);
    readGeneration.current++;
    setOptions(null);
  }, [state.revision, state.instance]);
  useEffect(() => {
    if (!loaded) return;
    const next = pendingRef.current.flatMap((p) => {
      const result = lanObservePending(
        p,
        state.instance,
        state.lan ?? [],
        state.updatedAt,
        connected,
        Date.now(),
      );
      return result ? [result] : [];
    });
    if (JSON.stringify(next) !== JSON.stringify(pendingRef.current))
      retainPending(next);
  }, [loaded, state.instance, state.lan, state.updatedAt, connected]);
  const announced = useRef(new Map<string, string>());
  useEffect(() => {
    const signature = (op: LANAccessStatus) =>
      `${state.instance}/${op.sequence}/${lanIngressFresh(op, state.updatedAt, connected, now)}`;
    const changes = (state.lan ?? []).filter(
      (op) => announced.current.get(op.operationId) !== signature(op),
    );
    announced.current = new Map(
      (state.lan ?? [])
        .slice(0, 128)
        .map((op) => [op.operationId, signature(op)]),
    );
    if (changes.length)
      setAnnouncement(
        changes
          .slice(-4)
          .map(
            (op) =>
              `Port ${op.selection.hostPort}: ${op.phase === "ready" && !lanIngressFresh(op, state.updatedAt, connected, now) ? "LAN state unconfirmed" : phaseText[op.phase]}. Connections ${op.ingress === "closed" ? "closed" : lanIngressFresh(op, state.updatedAt, connected, now) ? "ready" : "unconfirmed"}. ${firewallText[op.firewall]}.`,
          )
          .join(" ")
          .slice(0, 800),
      );
  }, [state.instance, state.lan, state.updatedAt, connected, now]);
  const readBusy = active.includes("read");
  const activationBlocked = !loaded || !!storageError || pending.length > 0;
  const disabled = busy || readBusy;
  const port = options?.publications.find(
    (p) => publicationKey(p) === publication,
  );
  const iface = options?.interfaces.find(
    (n) => `${n.interfaceGuid}/${n.interfaceLuid}` === network,
  );

  async function request(
    input: Parameters<typeof backend.localServices>[0],
    changes: boolean,
    lane = "read",
  ) {
    if (inFlight.current.has(lane) || (busy && lane === "read")) return null;
    inFlight.current.add(lane);
    const generation = readGeneration.current;
    if (lane === "read") setReadDismissed(false);
    setActive([...inFlight.current]);
    setError("");
    try {
      const response = await backend.localServices(input);
      if (
        !live.current ||
        (lane === "read" && generation !== readGeneration.current)
      )
        return null;
      receive(response);
      if (response.error) {
        setError(response.error);
        return null;
      }
      return response;
    } catch (cause) {
      if (
        live.current &&
        (lane !== "read" || generation === readGeneration.current)
      )
        setError(
          changes
            ? `The change response is unconfirmed. Use recovery controls; the request was not repeated. ${String(cause)}`
            : String(cause),
        );
      return null;
    } finally {
      inFlight.current.delete(lane);
      if (live.current) setActive([...inFlight.current]);
      if (changes && live.current) void refresh();
    }
  }
  function dismissRead() {
    readGeneration.current++;
    setReadDismissed(true);
    setReview(null);
  }
  async function readOptions() {
    setReview(null);
    const response = await request({ operation: "lan-options" }, false);
    if (!response) return null;
    const fresh = response.lanOptions;
    if (!fresh || !lanOptionsCurrent(fresh, Date.now())) {
      if (fresh?.version === 1) setOptions(fresh);
      setError(
        "Current complete LAN choices are unavailable. Read them again before reviewing access.",
      );
      return null;
    }
    const kept = lanRetainChoices(options, fresh, {
      publication,
      network,
      address,
    });
    setOptions(fresh);
    setPublication(kept.publication);
    setNetwork(kept.network);
    setAddress(kept.address);
    return { fresh, kept };
  }
  async function reviewAccess() {
    if (activationBlocked) return;
    let clientNetworks: string[];
    try {
      clientNetworks = lanClientNetworks(clients);
    } catch (cause) {
      setError(String(cause));
      return;
    }
    const original = { publication, network, address };
    const refreshed = await readOptions();
    if (!refreshed) return;
    if (
      Object.keys(original).some(
        (key) =>
          original[key as keyof typeof original] !==
          refreshed.kept[key as keyof typeof original],
      )
    ) {
      setError(
        "The selected port, network, or address changed. Choose its current identity before reviewing access.",
      );
      return;
    }
    const currentPort = refreshed.fresh.publications.find(
      (p) => publicationKey(p) === publication,
    );
    const currentInterface = refreshed.fresh.interfaces.find(
      (n) => `${n.interfaceGuid}/${n.interfaceLuid}` === network,
    );
    if (
      !currentPort?.available ||
      !currentInterface?.available ||
      !currentInterface.privateAddresses.includes(address) ||
      (currentInterface.profile !== "private" &&
        currentInterface.profile !== "domain")
    )
      return;
    const atRevision = revision.current;
    const atInstance = instance.current;
    const response = await request(
      {
        operation: "lan-review",
        revision: atRevision,
        lanSelection: {
          containerId: currentPort.containerId,
          startedAt: currentPort.startedAt,
          hostPort: currentPort.hostPort,
          containerPort: currentPort.containerPort,
          protocol: currentPort.protocol,
          interfaceGuid: currentInterface.interfaceGuid,
          interfaceLuid: currentInterface.interfaceLuid,
          localAddress: address,
          profile: currentInterface.profile,
          clientNetworks,
        },
      },
      false,
    );
    acceptReview(response?.lanReview, atRevision, atInstance, "activate");
  }
  function acceptReview(
    value: LANReview | undefined,
    atRevision: number,
    atInstance: string,
    purpose: LANReview["purpose"],
  ) {
    if (!value) return;
    if (
      revision.current !== atRevision ||
      instance.current !== atInstance ||
      value.purpose !== purpose
    ) {
      setError("The settings or review changed. Request a fresh review.");
      return;
    }
    if (!value.available) {
      setError(lanReason(value.reasonCode));
      return;
    }
    if (!lanReviewCurrent(value, Date.now())) {
      setError("The review is incomplete or expired. Request a fresh review.");
      return;
    }
    setReview({ value, revision: atRevision, runtimeInstance: atInstance });
  }
  async function reviewRemoval(lease: string, settle = false) {
    const atRevision = revision.current;
    const atInstance = instance.current;
    const response = await request(
      {
        operation: settle ? "lan-settle-remove-review" : "lan-remove-review",
        lanLeaseSha256: lease,
      },
      false,
    );
    acceptReview(
      response?.lanReview,
      atRevision,
      atInstance,
      settle ? "settle-remove" : "remove",
    );
  }
  async function reviewCleanup(operation: LANAccessStatus) {
    if (
      !/^[a-f0-9]{32}$/.test(operation.reviewId) ||
      !/^[a-f0-9]{64}$/.test(operation.reviewSha256 ?? "")
    ) {
      setError(
        "Read the protected cleanup record again before requesting its review.",
      );
      return;
    }
    const atRevision = revision.current,
      atInstance = instance.current;
    const response = await request(
      {
        operation: "lan-cleanup-review",
        lanReviewId: operation.reviewId,
        lanReviewSha256: operation.reviewSha256!,
      },
      false,
      `cleanup-review/${operation.operationId}`,
    );
    const purpose = response?.lanReview?.purpose;
    if (purpose !== "cleanup-remove" && purpose !== "cleanup-settle") {
      if (response?.lanReview)
        setError(
          "The cleanup review changed. Read its protected record again.",
        );
      return;
    }
    acceptReview(response?.lanReview, atRevision, atInstance, purpose);
  }
  async function reconcile(record: LANPendingChange) {
    const response = await request(
      {
        operation: "lan-reconcile-review",
        lanReviewId: record.reviewId,
        lanReviewSha256: record.reviewSha256,
      },
      false,
      `reconcile/${record.reviewId}`,
    );
    if (!response?.lanReconciliation || !response.state) return;
    const result = lanReconcilePending(
      record,
      response.state.instance,
      response.lanReconciliation,
    );
    retainPending(
      pendingRef.current.flatMap((p) =>
        p.runtimeInstance === record.runtimeInstance &&
        p.reviewId === record.reviewId
          ? result
            ? [result]
            : []
          : [p],
      ),
    );
    if (result)
      setError(
        "The original change remains retained or unconfirmed. Inspect its operation and read current status; no request was repeated.",
      );
    void refresh();
  }
  async function confirm() {
    const chosen = review;
    if (
      !chosen ||
      chosen.revision !== revision.current ||
      chosen.runtimeInstance !== instance.current ||
      !connected ||
      !lanReviewCurrent(chosen.value, Date.now()) ||
      (chosen.value.purpose === "activate" &&
        (activationBlocked || pendingRef.current.length > 0))
    )
      return;
    const lane = `submit/${chosen.value.reviewId}`;
    if (inFlight.current.has(lane)) return;
    const record: LANPendingChange = {
      runtimeInstance: chosen.runtimeInstance,
      reviewId: chosen.value.reviewId,
      reviewSha256: chosen.value.reviewSha256,
      purpose: chosen.value.purpose,
    };
    // Persist before dispatch. Failure denies new activation, but exact removal
    // recovery remains available and is still retained in memory.
    const previous = pendingRef.current;
    const persisted =
      previous.length < 16 && retainPending([...previous, record]);
    setReview(null);
    if (!persisted && chosen.value.purpose === "activate") {
      // Nothing was dispatched. Keep earlier unknown records and the storage
      // denial, without inventing an uncertain operation for this local failure.
      pendingRef.current = previous;
      setPending(previous);
      return;
    }
    if (!persisted)
      setError(
        "Removal recovery can continue, but this review could not be saved locally. Inspect the retained backend operation if its response is lost.",
      );
    await request(
      {
        operation: lanReviewOperation(chosen.value.purpose),
        revision: chosen.revision,
        lanReviewId: chosen.value.reviewId,
        lanReviewSha256: chosen.value.reviewSha256,
      },
      true,
      lane,
    );
  }
  return (
    <section aria-labelledby="lan-access-title">
      <h3 id="lan-access-title">LAN access</h3>
      <p>
        Share a published port with selected devices on a Private or Domain
        network. Localhost remains the default. Windows approval includes a
        separate review of the exact scope.
      </p>
      <div className="action-row">
        <button
          className="button"
          disabled={disabled}
          aria-busy={readBusy}
          onClick={() => void readOptions()}
        >
          {readBusy ? "Reading…" : "Read published ports and networks"}
        </button>
        <button
          className="button"
          disabled={busy}
          onClick={() => void refresh()}
        >
          Read current status
        </button>
      </div>
      {error && (
        <p className="notice" role="alert">
          {error}
        </p>
      )}
      <p role="status" aria-live="polite" aria-atomic="true" className="notice">
        {announcement}
      </p>
      {storageError && (
        <p className="notice" role="alert">
          {storageError}
        </p>
      )}
      {readBusy && (
        <div className="action-row">
          {!readDismissed ? (
            <button className="button" onClick={dismissRead}>
              Cancel reading result
            </button>
          ) : (
            <p role="status">
              Reading result dismissed. The underlying read may still finish;
              this did not cancel a LAN operation.
            </p>
          )}
        </div>
      )}
      {pending.length > 0 && (
        <div className="notice">
          <p role="status">
            A previous LAN change is awaiting confirmation. New activation is
            blocked. Read current status or reconcile the original review;
            requests are never repeated.
          </p>
          {pending.slice(0, 16).map((record) => (
            <div
              className="action-row"
              key={`${record.runtimeInstance}/${record.reviewId}`}
            >
              <span>
                {record.purpose === "activate"
                  ? "Activation"
                  : record.purpose === "settle-remove"
                    ? "Removal settlement"
                    : "Removal"}{" "}
                review <code>{record.reviewId}</code>
                {record.runtimeInstance !== state.instance
                  ? " (previous backend session; outcome unconfirmed)"
                  : ""}
              </span>
              <button
                className="button"
                disabled={active.includes(`reconcile/${record.reviewId}`)}
                onClick={() => void reconcile(record)}
              >
                Reconcile original review
              </button>
            </div>
          ))}
        </div>
      )}
      {!connected && (
        <p className="notice" role="status">
          The connection is unavailable. Current LAN access is unconfirmed.
        </p>
      )}
      {options && (
        <>
          {!options.sourceAvailable && (
            <p className="notice">
              {lanReason(options.sourceUnavailableReason)}
            </p>
          )}
          {!options.helperAvailable && (
            <p className="notice">
              {lanReason(options.helperUnavailableReason)}
            </p>
          )}
          {options.complete && !options.publications.length && (
            <p>No eligible published ports were found.</p>
          )}
          <div className="action-row">
            <label>
              Published port{" "}
              <select
                aria-label="LAN published port"
                value={publication}
                disabled={disabled || !connected}
                onChange={(event) => {
                  setPublication(event.target.value);
                  setReview(null);
                }}
              >
                <option value="">Choose a published port</option>
                {options.publications.map((p) => (
                  <option
                    key={publicationKey(p)}
                    value={publicationKey(p)}
                    disabled={!p.available}
                  >
                    {p.containerName || p.containerId.slice(0, 12)} —{" "}
                    {p.hostPort}/{p.protocol.toUpperCase()} → {p.containerPort}
                    {!p.available ? " (unavailable)" : ""}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Network connection{" "}
              <select
                aria-label="LAN network connection"
                value={network}
                disabled={disabled || !connected}
                onChange={(event) => {
                  setNetwork(event.target.value);
                  setAddress("");
                  setReview(null);
                }}
              >
                <option value="">Choose a network connection</option>
                {options.interfaces.map((n) => (
                  <option
                    key={`${n.interfaceGuid}/${n.interfaceLuid}`}
                    value={`${n.interfaceGuid}/${n.interfaceLuid}`}
                    disabled={!n.available}
                  >
                    {n.alias} — {n.profile}
                    {!n.available ? " (unavailable)" : ""}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Local address{" "}
              <select
                aria-label="LAN local address"
                value={address}
                disabled={disabled || !iface?.available || !connected}
                onChange={(event) => {
                  setAddress(event.target.value);
                  setReview(null);
                }}
              >
                <option value="">Choose an address</option>
                {iface?.privateAddresses.map((ip) => (
                  <option key={ip} value={ip}>
                    {ip}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <label>
            Allowed client ranges{" "}
            <input
              aria-label="LAN allowed client ranges"
              value={clients}
              maxLength={512}
              placeholder="192.168.1.0/24"
              disabled={disabled || !connected}
              onChange={(event) => {
                setClients(event.target.value);
                setReview(null);
              }}
            />
          </label>
          <p>
            Enter one to eight private IPv4 ranges, separated by commas. Only
            devices in these ranges may connect.
          </p>
          <button
            className="button"
            disabled={
              disabled ||
              activationBlocked ||
              !connected ||
              !options.complete ||
              !options.sourceAvailable ||
              !options.helperAvailable ||
              !port?.available ||
              !iface?.available ||
              !address ||
              !clients.trim()
            }
            onClick={() => void reviewAccess()}
          >
            Review LAN access…
          </button>
        </>
      )}
      {!state.lan?.length ? (
        <p>No retained LAN operations.</p>
      ) : (
        <ul>
          {state.lan.map((operation) => {
            const fresh = lanIngressFresh(
              operation,
              state.updatedAt,
              connected,
              now,
            );
            const lease = operation.leaseSha256;
            const validLease = /^[a-f0-9]{64}$/.test(lease ?? "");
            const validOperation = /^[a-f0-9]{32}$/.test(operation.operationId);
            return (
              <li key={operation.operationId}>
                <p>
                  <strong>
                    {operation.phase === "ready" && !fresh
                      ? "LAN state unconfirmed"
                      : phaseText[operation.phase]}
                  </strong>{" "}
                  {operation.selection.localAddress}:
                  {operation.selection.hostPort}/
                  {operation.selection.protocol.toUpperCase()}{" "}
                  <code>{operation.selection.containerId.slice(0, 12)}</code>
                </p>
                <p>
                  Connections:{" "}
                  {operation.ingress === "closed"
                    ? "Closed"
                    : fresh
                      ? "Ready"
                      : "Unconfirmed"}
                  . Firewall: {firewallText[operation.firewall]}.
                </p>
                {operation.reasonCode && (
                  <p>{lanReason(operation.reasonCode)}</p>
                )}
                {operation.error && (
                  <p className="notice" role="alert">
                    {operation.error}
                  </p>
                )}
                <div className="action-row">
                  {operation.canCancel && validOperation && (
                    <button
                      className="button"
                      disabled={active.includes(
                        `cancel/${operation.operationId}`,
                      )}
                      onClick={() =>
                        void request(
                          {
                            operation: "lan-cancel",
                            lanOperationId: operation.operationId,
                          },
                          true,
                          `cancel/${operation.operationId}`,
                        )
                      }
                    >
                      Cancel setup
                    </button>
                  )}
                  {operation.canInspect && validLease && (
                    <button
                      className="button"
                      disabled={active.includes(
                        `inspect/${operation.operationId}`,
                      )}
                      onClick={() =>
                        void request(
                          { operation: "lan-inspect", lanLeaseSha256: lease! },
                          false,
                          `inspect/${operation.operationId}`,
                        )
                      }
                    >
                      Inspect retained state
                    </button>
                  )}
                  {operation.canReviewRemoval && validLease && (
                    <button
                      className="button"
                      disabled={disabled || !connected}
                      onClick={() => void reviewRemoval(lease!)}
                    >
                      Review removal…
                    </button>
                  )}
                  {operation.canSettleRemoval && validLease && (
                    <button
                      className="button"
                      disabled={disabled || !connected}
                      onClick={() => void reviewRemoval(lease!, true)}
                    >
                      Review removal settlement…
                    </button>
                  )}
                  {operation.canReviewCleanup && validLease && (
                    <button
                      className="button"
                      disabled={
                        disabled ||
                        !connected ||
                        active.includes(
                          `cleanup-review/${operation.operationId}`,
                        )
                      }
                      onClick={() => void reviewCleanup(operation)}
                    >
                      Review retained cleanup…
                    </button>
                  )}
                  {operation.phase === "cleanup-pending" && validOperation && (
                    <button
                      className="button"
                      disabled={active.includes(
                        `settle/${operation.operationId}`,
                      )}
                      onClick={() =>
                        void request(
                          {
                            operation: "lan-settle-helper",
                            lanOperationId: operation.operationId,
                          },
                          false,
                          `settle/${operation.operationId}`,
                        )
                      }
                    >
                      Recheck cleanup
                    </button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
      {review && (
        <ConfirmDialog
          title={
            review.value.purpose === "activate"
              ? "Allow LAN access?"
              : review.value.purpose === "settle-remove"
                ? "Finish recording removal?"
                : review.value.purpose === "cleanup-remove"
                  ? "Remove the retained LAN rule?"
                  : review.value.purpose === "cleanup-settle"
                    ? "Confirm retained removal?"
                    : "Close LAN access and remove its rule?"
          }
          confirmLabel={
            review.value.purpose === "activate"
              ? "Continue to Windows approval"
              : review.value.purpose === "settle-remove"
                ? "Verify removal and finish"
                : review.value.purpose === "cleanup-remove" ||
                    review.value.purpose === "cleanup-settle"
                  ? "Continue to Windows cleanup review"
                  : "Close access and continue"
          }
          destructive={
            review.value.purpose === "remove" ||
            review.value.purpose === "cleanup-remove"
          }
          confirmDisabled={
            busy ||
            (review.value.purpose === "activate" && activationBlocked) ||
            !connected ||
            review.revision !== state.revision ||
            review.runtimeInstance !== state.instance ||
            !lanReviewCurrent(review.value, now)
          }
          onCancel={() => setReview(null)}
          onConfirm={() => void confirm()}
        >
          <dl className="properties">
            <dt>Container</dt>
            <dd>
              <code>{review.value.selection.containerId}</code>
            </dd>
            <dt>Address and port</dt>
            <dd>
              {review.value.selection.localAddress}:
              {review.value.selection.hostPort}/
              {review.value.selection.protocol.toUpperCase()}
            </dd>
            <dt>Container port</dt>
            <dd>{review.value.selection.containerPort}</dd>
            <dt>Network profile</dt>
            <dd>{review.value.selection.profile}</dd>
            <dt>Client ranges</dt>
            <dd>{review.value.selection.clientNetworks.join(", ")}</dd>
            <dt>Publication generation</dt>
            <dd>{review.value.publicationGeneration}</dd>
          </dl>
          {review.value.purpose === "activate" ? (
            <p>
              Application data on this port becomes reachable from the selected
              devices. Windows will ask for administrator approval and show the
              exact scope for confirmation.
            </p>
          ) : review.value.purpose === "cleanup-remove" ||
            review.value.purpose === "cleanup-settle" ? (
            <p>
              The earlier LAN owner has exited. Windows will verify the original
              protected rule and its closed resources before cleanup. This
              cannot reopen access or replay the earlier request.
            </p>
          ) : review.value.purpose === "settle-remove" ? (
            <p>
              The earlier removal was not fully acknowledged. Windows approval
              will only verify that the rule is already absent and finish its
              recovery record. It will not repeat the delete request.
            </p>
          ) : (
            <p>
              OrbWin closes this LAN listener and its connections before
              retiring the exact owned rule. Denied or uncertain cleanup retains
              its record for inspection.
            </p>
          )}
          {!lanReviewCurrent(review.value, now) && (
            <p role="alert">
              The review expired. Cancel and request a fresh review.
            </p>
          )}
        </ConfirmDialog>
      )}
    </section>
  );
}
