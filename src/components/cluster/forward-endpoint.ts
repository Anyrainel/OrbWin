export type ForwardProtocol = "tcp" | "http" | "https";

export function forwardEndpoint(
  local: number,
  remote: number,
  protocol: ForwardProtocol,
): { address: string; url: string | null } | null {
  if (
    !Number.isInteger(local) ||
    local < 1 ||
    local > 65535 ||
    !Number.isInteger(remote) ||
    remote < 1 ||
    remote > 65535 ||
    !["tcp", "http", "https"].includes(protocol)
  )
    return null;
  const address = `127.0.0.1:${local}`;
  return {
    address,
    url: protocol === "tcp" ? null : `${protocol}://${address}/`,
  };
}
