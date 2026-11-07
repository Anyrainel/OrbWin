import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { ConfirmDialog } from "../shell/ConfirmDialog";

type NetworkChoice = { id: string; name: string };

export function LocalServiceNetworks({
  selected,
  busy,
  save,
}: {
  selected: string[];
  busy: boolean;
  save: (ids: string[]) => Promise<void>;
}) {
  const [networks, setNetworks] = useState<NetworkChoice[] | null>(null);
  const [draft, setDraft] = useState(selected);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [review, setReview] = useState(false);
  const live = useRef(true);
  const savedKey = selected.join(",");
  useEffect(() => {
    setDraft(savedKey ? savedKey.split(",") : []);
    setReview(false);
  }, [savedKey]);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);
  async function read() {
    if (loading) return;
    setLoading(true);
    setError("");
    try {
      const snapshot = await backend.inventory();
      const choices = snapshot.networks
        .filter(
          (row) =>
            typeof row.ID === "string" &&
            /^[a-f0-9]{64}$/.test(row.ID) &&
            row.Driver === "bridge" &&
            row.Scope === "local" &&
            !["bridge", "host", "none"].includes(String(row.Name)),
        )
        .map((row) => ({ id: String(row.ID), name: String(row.Name) }));
      choices.sort(
        (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
      );
      if (live.current) setNetworks(choices);
    } catch (cause) {
      if (live.current) setError(String(cause));
    } finally {
      if (live.current) setLoading(false);
    }
  }
  const visible = [...(networks ?? [])];
  for (const id of draft)
    if (!visible.some((network) => network.id === id))
      visible.push({ id, name: "Saved selection (not in the current read)" });
  const dirty = [...draft].sort().join(",") !== savedKey;
  return (
    <section aria-labelledby="local-service-networks-title" aria-busy={loading}>
      <h3 id="local-service-networks-title">Container clients</h3>
      <p>
        Choose up to eight user-defined bridge networks whose clients may reach
        all active local HTTP services through <code>*.orbwin.test</code>.
        OrbWin joins its relay to these networks. Workload connections stay as
        they are.
      </p>
      <p>
        Clients must use the reviewed public root for HTTPS verification.
        Selecting a network does not change container trust stores.
      </p>
      <button
        className="button"
        disabled={busy || loading}
        onClick={() => void read()}
      >
        {loading ? "Reading networks…" : "Read available networks"}
      </button>
      {error && (
        <p className="notice" role="alert">
          {error}
        </p>
      )}
      {networks !== null && !networks.length && (
        <p>No user-defined local bridge networks were found.</p>
      )}
      {visible.map((network) => (
        <label className="settings-network-option" key={network.id}>
          <input
            type="checkbox"
            checked={draft.includes(network.id)}
            disabled={
              busy ||
              loading ||
              (!draft.includes(network.id) && draft.length >= 8)
            }
            onChange={(event) =>
              setDraft((ids) =>
                event.target.checked
                  ? [...ids, network.id].sort()
                  : ids.filter((id) => id !== network.id),
              )
            }
          />{" "}
          {network.name} <code>{network.id.slice(0, 12)}</code>
        </label>
      ))}
      <p>
        {selected.length
          ? `${selected.length} saved client network${selected.length === 1 ? "" : "s"}.`
          : "No container-client networks selected."}
      </p>
      <button
        className="button"
        disabled={busy || loading || !dirty}
        onClick={() => setReview(true)}
      >
        Review network access…
      </button>
      {review && (
        <ConfirmDialog
          title="Change container-client access?"
          confirmLabel="Save network access"
          confirmDisabled={busy || loading}
          onCancel={() => setReview(false)}
          onConfirm={() => {
            setReview(false);
            void save([...draft].sort());
          }}
        >
          <p>
            Clients on these networks may reach every active OrbWin local HTTP
            service through the relay, including services whose workloads use
            another network:
          </p>
          {draft.length ? (
            <ul>
              {draft.map((id) => (
                <li key={id}>
                  {visible.find((network) => network.id === id)?.name}{" "}
                  <code>{id}</code>
                </li>
              ))}
            </ul>
          ) : (
            <p>
              No additional client networks. Existing selected networks will be
              disconnected from the relay.
            </p>
          )}
          <p>
            Applying this choice restarts the local relay and briefly interrupts
            its connections. It does not restart workloads or install trusted
            certificates.
          </p>
        </ConfirmDialog>
      )}
    </section>
  );
}
