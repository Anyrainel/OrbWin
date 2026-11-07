import { invoke, isTauri } from "@tauri-apps/api/core";
import type { ClientRecoveryReview } from "./backend";

export type ClientBootstrapSelection =
  | "empty"
  | "import-current-source"
  | "resume-import";

// A closed action only. Paths, source bytes and authority never come from UI.
export async function reviewClientBootstrap(
  selection: ClientBootstrapSelection,
): Promise<ClientRecoveryReview | null> {
  if (!isTauri())
    throw new Error("Private settings setup requires the Windows desktop application.");
  return invoke<ClientRecoveryReview | null>("client_bootstrap_review", {
    selection,
  });
}
