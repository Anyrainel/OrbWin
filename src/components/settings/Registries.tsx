import { useEffect, useRef, useState, type FormEvent } from "react";
import { isTauri } from "@tauri-apps/api/core";
import * as backend from "../../backend";
import { retryBusyRead } from "../../model/read-retry";
import { ConfirmDialog } from "../shell/ConfirmDialog";

type Saved = { server: string; username: string };
export function Registries() {
  const [server, setServer] = useState("docker.io");
  const [username, setUsername] = useState("");
  const [secret, setSecret] = useState("");
  const [saved, setSaved] = useState<Saved[] | null>(null);
  const [readError, setReadError] = useState("");
  const [reading, setReading] = useState(false);
  const [busy, setBusy] = useState<"login" | "forget" | null>(null);
  const [message, setMessage] = useState("");
  const [confirm, setConfirm] = useState<Saved | null>(null);
  const active = useRef<AbortController | null>(null);
  const read = useRef<AbortController | null>(null);
  const generation = useRef(0);
  const mounted = useRef(false);
  const desktop = isTauri();

  async function refresh() {
    read.current?.abort();
    const abort = new AbortController();
    read.current = abort;
    const current = ++generation.current;
    setReading(true);
    setReadError("");
    try {
      let text = "";
      const reply = await retryBusyRead(() => {
        text = "";
        return backend.run(
          crypto.randomUUID(),
          { kind: "registryList" },
          (chunk) => {
            text += chunk;
          },
          abort.signal,
        );
      }, abort.signal);
      if (!reply.success) throw new Error(reply.message);
      const value: unknown = JSON.parse(text);
      if (
        !value ||
        typeof value !== "object" ||
        Array.isArray(value) ||
        Object.keys(value).length > 256 ||
        Object.entries(value).some(
          ([server, user]) =>
            server.length > 2048 ||
            typeof user !== "string" ||
            user.length > 512,
        )
      )
        throw new Error("The saved registry list has an invalid shape.");
      if (
        mounted.current &&
        current === generation.current &&
        !abort.signal.aborted
      )
        setSaved(
          Object.entries(value)
            .map(([server, username]) => ({
              server,
              username: String(username),
            }))
            .sort((a, b) => a.server.localeCompare(b.server)),
        );
    } catch (error) {
      if (
        mounted.current &&
        current === generation.current &&
        !abort.signal.aborted
      )
        setReadError(String(error));
    } finally {
      if (mounted.current && current === generation.current) setReading(false);
      if (read.current === abort) read.current = null;
    }
  }
  useEffect(() => {
    mounted.current = true;
    void refresh();
    return () => {
      mounted.current = false;
      generation.current++;
      read.current?.abort();
      active.current?.abort();
    };
  }, []);

  async function perform(
    request: Extract<
      backend.Operation,
      { kind: "registryLogin" | "registryForget" }
    >,
  ) {
    if (active.current) return;
    const abort = new AbortController();
    active.current = abort;
    setBusy(request.kind === "registryLogin" ? "login" : "forget");
    setMessage("");
    setSecret("");
    try {
      // Login output is deliberately discarded natively. A registry's error
      // response may echo supplied credentials; show only the safe outcome.
      const reply = await backend.run(
        crypto.randomUUID(),
        request,
        () => {},
        abort.signal,
      );
      if (mounted.current) setMessage(reply.message);
    } catch (error) {
      if (mounted.current) setMessage(String(error));
    } finally {
      if (active.current === abort) active.current = null;
      if (mounted.current) {
        setBusy(null);
        await refresh();
      }
    }
  }
  function login(event: FormEvent) {
    event.preventDefault();
    if (!desktop || busy || !username || !secret) return;
    void perform({ kind: "registryLogin", server, username, secret });
  }
  return (
    <>
      <h2>Registry credentials</h2>
      <p>
        Sign in for private image pulls and builds. Credentials are saved in
        Windows Credential Manager under OrbWin's own entries. Passwords and
        tokens are never included in diagnostic exports.
      </p>
      <p className="settings-hint">
        Saved entries are used by OrbWin pulls, builds and Compose operations.
        Connected WSL clients use the command shown for their distribution and
        require Windows interop. External clients using their own Docker config
        keep their own credentials.
      </p>
      {!desktop && (
        <p role="status">
          Registry credentials require the Windows desktop application.
        </p>
      )}
      <form className="registry-form" onSubmit={login}>
        <fieldset disabled={!desktop || busy !== null}>
          <label>
            Registry
            <input
              value={server}
              onChange={(event) => setServer(event.target.value)}
              autoComplete="off"
              spellCheck={false}
              placeholder="docker.io or registry.example:5000"
              maxLength={253}
            />
          </label>
          <label>
            Username
            <input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="off"
              spellCheck={false}
              required
              maxLength={512}
            />
          </label>
          <label>
            Password or access token
            <input
              type="password"
              value={secret}
              onChange={(event) => setSecret(event.target.value)}
              autoComplete="off"
              required
              maxLength={2560}
            />
          </label>
          <button type="submit" className="button primary">
            Sign in
          </button>
        </fieldset>
      </form>
      {busy && (
        <div className="action-row">
          <span role="status">
            {busy === "login"
              ? "Authenticating and saving credentials…"
              : "Removing saved credentials…"}
          </span>
          <button className="button" onClick={() => active.current?.abort()}>
            Cancel
          </button>
        </div>
      )}
      {message && <p role="status">{message}</p>}
      <p className="settings-hint">
        Cancelling or leaving this page stops the client. Credentials may
        already have been saved; the list is refreshed after each attempt.
        Tokens are limited to 2560 bytes by the Windows store.
      </p>
      <h3>Saved in OrbWin</h3>
      <div className="action-row">
        <button
          className="button"
          disabled={!desktop || reading || busy !== null}
          onClick={() => void refresh()}
        >
          Refresh
        </button>
        {reading && <span role="status">Reading saved registries…</span>}
      </div>
      {readError && <p role="alert">{readError} Use Refresh to try again.</p>}
      {saved?.length === 0 && (
        <p>No registry credentials are saved in OrbWin.</p>
      )}
      {saved && saved.length > 0 && (
        <table className="registry-table">
          <thead>
            <tr>
              <th>Registry</th>
              <th>Username</th>
              <th>Action</th>
            </tr>
          </thead>
          <tbody>
            {saved.map((item) => (
              <tr key={item.server}>
                <td>
                  {item.server === "https://index.docker.io/v1/"
                    ? "Docker Hub"
                    : item.server}
                </td>
                <td>{item.username}</td>
                <td>
                  <button
                    className="button danger"
                    disabled={busy !== null}
                    onClick={() => setConfirm(item)}
                    aria-label={`Forget OrbWin credentials for ${item.server}`}
                  >
                    Forget
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {confirm && (
        <ConfirmDialog
          title={`Forget credentials for ${confirm.server}?`}
          confirmLabel="Forget credentials"
          destructive
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            const selected = confirm;
            setConfirm(null);
            void perform({ kind: "registryForget", server: selected.server });
          }}
        >
          <p>
            Removes this registry's saved credentials from OrbWin's Windows
            store. Future private pulls and builds may need another sign-in.
            Downloaded images and running workloads are retained.
          </p>
        </ConfirmDialog>
      )}
    </>
  );
}
