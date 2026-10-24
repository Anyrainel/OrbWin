export function assertOwnedChanges(changed, permitted) {
  const allowed = new Set(permitted);
  const unexpected = changed.filter((path) => !allowed.has(path));
  if (unexpected.length)
    throw new Error(
      `Upstream has unrelated changes; preserve them before preparing: ${unexpected.join(", ")}`,
    );
}
