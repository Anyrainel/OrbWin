// Classify only returned Kubernetes ownership labels, never a name heuristic.
// This is a reversible display filter; the complete inventory stays authoritative.
export function isKubernetesContainer(row: Record<string, unknown>): boolean {
  const labels = row.Labels;
  if (typeof labels === "string")
    return /(?:^|,)io\.kubernetes\.pod\.uid=[^,]+/.test(labels);
  return !!(
    labels &&
    typeof labels === "object" &&
    typeof (labels as Record<string, unknown>)["io.kubernetes.pod.uid"] ===
      "string"
  );
}
