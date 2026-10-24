function interval(cidr) {
  const [address, mask] = cidr.split("/");
  const prefix = Number(mask);
  const bytes = address.split(".").map(Number);
  if (
    bytes.length !== 4 ||
    bytes.some((byte) => !Number.isInteger(byte) || byte < 0 || byte > 255) ||
    !Number.isInteger(prefix) ||
    prefix < 0 ||
    prefix > 32
  )
    throw new Error(`Invalid host route: ${cidr}`);
  const ip = bytes.reduce((value, byte) => value * 256 + byte, 0);
  const size = 2 ** (32 - prefix);
  const start = Math.floor(ip / size) * size;
  return [start, start + size - 1];
}
export function conflictingRoutes(
  routes,
  candidates = ["192.168.144.0/24", "192.168.127.0/24"],
) {
  return routes
    .filter((route) => route !== "0.0.0.0/0")
    .filter((route) => {
      const [start, end] = interval(route);
      return candidates.some((candidate) => {
        const [otherStart, otherEnd] = interval(candidate);
        return start <= otherEnd && otherStart <= end;
      });
    });
}
