import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import type { ClusterKind, K8sObject } from "./model";
import { parseList } from "./model";

export const CANCELLED = "Read cancelled. Cluster workloads were not stopped.";

// One bounded kubectl invocation through the existing operation adapter. Aborting
// discards the result and asks the native layer to cancel; it never changes
// workloads.
export async function kubectl(
  request: backend.Operation,
  signal: AbortSignal,
): Promise<string> {
  let output = "";
  const outcome = await backend.run(
    crypto.randomUUID(),
    request,
    (chunk) => {
      output += chunk;
    },
    signal,
  );
  if (signal.aborted) throw new Error(CANCELLED);
  if (!outcome.success)
    throw new Error(`${outcome.message}${output ? `\n${output}` : ""}`);
  return output;
}
export async function listObjects(
  resource: ClusterKind | "events",
  namespace: string,
  signal: AbortSignal,
): Promise<K8sObject[]> {
  return parseList(
    await kubectl(
      { kind: "cluster", action: "list", resource, namespace, name: "" },
      signal,
    ),
  );
}

export type ReadState = {
  status: "idle" | "loading" | "done" | "error";
  text: string;
  error: string;
};
// Loads text while enabled. Re-runs when `key` changes or reload() is called.
export function useRead(
  load: (signal: AbortSignal) => Promise<string>,
  key: string,
  enabled: boolean,
) {
  const [state, setState] = useState<ReadState>({
    status: "idle",
    text: "",
    error: "",
  });
  const [nonce, setNonce] = useState(0);
  const loader = useRef(load);
  loader.current = load;
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    if (!enabled) return;
    const abort = new AbortController();
    controller.current = abort;
    setState({ status: "loading", text: "", error: "" });
    loader
      .current(abort.signal)
      .then((text) => {
        if (!abort.signal.aborted) setState({ status: "done", text, error: "" });
      })
      .catch((error) => {
        if (!abort.signal.aborted)
          setState({
            status: "error",
            text: "",
            error: error instanceof Error ? error.message : String(error),
          });
      });
    return () => abort.abort();
  }, [key, enabled, nonce]);
  return {
    ...state,
    reload: () => setNonce((n) => n + 1),
    cancel: () => {
      controller.current?.abort();
      setState({ status: "error", text: "", error: CANCELLED });
    },
  };
}

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
