// Kubernetes metadata contains server bookkeeping and potentially sensitive
// annotations. A typed view does not remove extra runtime keys: select the
// operation target explicitly before crossing the strict native boundary.
export function clusterTarget(metadata: { name: string; namespace: string }) {
  return { name: metadata.name, namespace: metadata.namespace };
}

export function clusterDeletionTarget(metadata: {
  name: string;
  namespace: string;
  uid: string;
}) {
  return { ...clusterTarget(metadata), uid: metadata.uid };
}
