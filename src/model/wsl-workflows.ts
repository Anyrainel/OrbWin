import type { Operation } from "../backend";

type Compose = Extract<Operation, { kind: "wslCompose" }>;
// The source argument is the displayed/remembered project root. Caller hints
// cannot replace its authority; copy lists so later inventory edits cannot alter
// an already-open operation. Native preparation rechecks this root before use.
export function wslComposeRequest(
  distribution: string,
  source: string,
  action: Compose["action"],
  inputs?: Partial<Compose> & { signature?: string },
): Compose {
  if (inputs?.connection && inputs.connection.source !== source)
    throw new Error(
      "The reviewed WSL connection and project root differ. Inspect the project again.",
    );
  const signature = inputs?.connection?.signature ?? inputs?.signature;
  if (!signature || signature.length > 64 || !/^[0-9]+:[0-9]+$/.test(signature))
    throw new Error(
      "Inspect the current WSL project connection before reopening its workflows.",
    );
  return {
    kind: "wslCompose",
    distribution,
    connection: { ...(inputs?.connection ?? {}), source, signature },
    file: inputs?.file ?? "compose.yaml",
    project:
      inputs?.project ??
      `orbwin-${
        source
          .split("/")
          .filter(Boolean)
          .at(-1)
          ?.toLowerCase()
          .replace(/[^a-z0-9_-]/g, "-")
          .replace(/^-+/, "") || "project"
      }`,
    directory: inputs?.directory ?? "",
    overrides: [...(inputs?.overrides ?? [])],
    envFiles: [...(inputs?.envFiles ?? [])],
    profiles: [...(inputs?.profiles ?? [])],
    services: ["down", "validate"].includes(action)
      ? []
      : [...(inputs?.services ?? [])],
    action,
  };
}
