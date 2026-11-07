export function newLocalServiceSnapshotOrder() {
  return { instance: "", sequence: 0, retired: new Set<string>() };
}

// Wall time is display metadata. A late response from a prior process or an
// older observation must not undo a completed action in the settings panel.
export function acceptLocalServiceSnapshot(
  order: ReturnType<typeof newLocalServiceSnapshotOrder>,
  snapshot: { instance: string; sequence: number },
) {
  const { instance, sequence } = snapshot;
  if (
    !/^[a-f0-9]{32}$/.test(instance) ||
    !Number.isSafeInteger(sequence) ||
    sequence < 1
  ) {
    throw new Error(
      "Local service snapshot ordering metadata is unsupported; rebuild or restart its component.",
    );
  }
  if (order.retired.has(instance)) return false;
  if (instance === order.instance && sequence < order.sequence) return false;
  if (order.instance && instance !== order.instance)
    order.retired.add(order.instance);
  order.instance = instance;
  order.sequence = sequence;
  return true;
}
