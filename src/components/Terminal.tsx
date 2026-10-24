import { useEffect, useRef, useState } from "react";
import { Terminal as Xterm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { Channel, invoke } from "@tauri-apps/api/core";
import "@xterm/xterm/css/xterm.css";

type Event =
  | { kind: "ready" }
  | { kind: "data"; bytes: number[] }
  | { kind: "exit"; code: number | null; message: string };
export function Terminal({ target }: { target: string }) {
  const host = useRef<HTMLDivElement>(null);
  const terminal = useRef<Xterm | null>(null);
  const fit = useRef<FitAddon | null>(null);
  const session = useRef<string | null>(null);
  const wanted = useRef(false);
  const mounted = useRef(true);
  const [connected, setConnected] = useState(false);
  const [pending, setPending] = useState(false);
  const [shell, setShell] = useState("sh");
  const [message, setMessage] = useState("");
  const inputQueue = useRef(Promise.resolve());
  const queued = useRef(0);
  useEffect(() => {
    mounted.current = true;
    const term = new Xterm({
      fontFamily: "Consolas, monospace",
      fontSize: 12,
      scrollback: 2000,
      disableStdin: true,
      screenReaderMode: true,
      theme: {
        background: "#1f1c20",
        foreground: "#e8e5e9",
        cursor: "#e8e5e9",
      },
    });
    const addon = new FitAddon();
    term.loadAddon(addon);
    term.open(host.current!);
    terminal.current = term;
    fit.current = addon;
    const observer = new ResizeObserver(() => {
      addon.fit();
    });
    observer.observe(host.current!);
    addon.fit();
    const input = term.onData((data) => {
      const id = session.current;
      if (!id || !wanted.current) return;
      const bytes = new TextEncoder().encode(data);
      if (queued.current + bytes.length > 65536) {
        setMessage("Input queue is full. Wait before pasting more text.");
        return;
      }
      queued.current += bytes.length;
      inputQueue.current = inputQueue.current
        .then(async () => {
          if (session.current !== id || !wanted.current) return;
          for (let offset = 0; offset < bytes.length; offset += 8192)
            await invoke("terminal_input", {
              id,
              bytes: Array.from(bytes.subarray(offset, offset + 8192)),
            });
        })
        .catch((error) => {
          if (mounted.current) setMessage(String(error));
        })
        .finally(() => {
          queued.current -= bytes.length;
        });
    });
    let resizeTimer = 0;
    const resize = term.onResize(({ cols, rows }) => {
      window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => {
        const id = session.current;
        if (id && wanted.current)
          inputQueue.current = inputQueue.current
            .then(async () => {
              if (session.current !== id || !wanted.current) return;
              await invoke("terminal_resize", {
                id,
                cols: Math.max(10, Math.min(400, cols)),
                rows: Math.max(2, Math.min(150, rows)),
              });
            })
            .catch((error) => {
              if (mounted.current && session.current === id)
                setMessage(String(error));
            });
      }, 100);
    });
    return () => {
      mounted.current = false;
      wanted.current = false;
      const id = session.current;
      session.current = null;
      if (id) void invoke("terminal_close", { id }).catch(() => {});
      window.clearTimeout(resizeTimer);
      observer.disconnect();
      input.dispose();
      resize.dispose();
      term.dispose();
      terminal.current = null;
    };
  }, []);
  async function open() {
    if (pending || connected || !terminal.current) return;
    const id = crypto.randomUUID();
    session.current = id;
    wanted.current = true;
    setPending(true);
    setMessage("");
    const term = terminal.current;
    term.clear();
    fit.current?.fit();
    const output = new Channel<Event>();
    output.onmessage = (event) => {
      if (event.kind === "ready") {
        if (mounted.current && wanted.current && session.current === id) {
          term.options.disableStdin = false;
          setConnected(true);
          setPending(false);
          term.focus();
        }
      } else if (event.kind === "data") {
        if (mounted.current && session.current === id)
          term.write(new Uint8Array(event.bytes), () => {
            void invoke("terminal_ack", {
              id,
              bytes: event.bytes.length,
            }).catch(() => {});
          });
      } else if (mounted.current && session.current === id) {
        wanted.current = false;
        session.current = null;
        term.options.disableStdin = true;
        setConnected(false);
        setPending(false);
        setMessage(`${event.message} Exit code: ${event.code ?? "unknown"}.`);
      }
    };
    try {
      await invoke("terminal_open", {
        id,
        container: target,
        shell,
        cols: Math.max(10, Math.min(400, term.cols)),
        rows: Math.max(2, Math.min(150, term.rows)),
        output,
      });
      if (!mounted.current || !wanted.current || session.current !== id) {
        await invoke("terminal_close", { id }).catch(() => {});
        return;
      }
    } catch (error) {
      if (mounted.current) {
        setMessage(String(error));
        session.current = null;
        wanted.current = false;
        setPending(false);
      }
    }
  }
  function close() {
    wanted.current = false;
    if (terminal.current) terminal.current.options.disableStdin = true;
    const id = session.current;
    if (id)
      void invoke("terminal_close", { id }).catch((error) => {
        if (mounted.current && !pending) setMessage(String(error));
      });
    setMessage(
      "Disconnect requested. The container remains running; an exec process may remain inside it.",
    );
  }
  return (
    <section>
      <div className="action-row">
        <label>
          Shell{" "}
          <select
            aria-label="Container shell"
            value={shell}
            disabled={pending || connected}
            onChange={(e) => setShell(e.target.value)}
          >
            <option value="sh">/bin/sh</option>
            <option value="bash">/bin/bash</option>
          </select>
        </label>
        <button
          className="button"
          disabled={pending || connected}
          onClick={() => void open()}
        >
          {pending ? "Connecting…" : "Open shell"}
        </button>
        {(connected || pending) && (
          <button className="button" onClick={close}>
            Disconnect
          </button>
        )}
      </div>
      <p role="status">
        {message ||
          (connected
            ? "Connected to the selected container. Exit the shell to end the exec process."
            : "Open an interactive shell in this container. The image must contain the selected shell.")}
      </p>
      <div
        className="terminal-host"
        ref={host}
        onKeyDown={(event) => event.stopPropagation()}
        aria-label="Interactive container terminal"
      />
    </section>
  );
}
