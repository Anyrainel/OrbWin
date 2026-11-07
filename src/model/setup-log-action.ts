// Native errors are fixed DATA codes, never raw paths/provider diagnostics.
export function setupLogErrorMessage(error: unknown): string {
  if (
    typeof error !== "object" ||
    error === null ||
    Array.isArray(error) ||
    Object.keys(error).length !== 1 ||
    !("code" in error)
  )
    return "Setup logs could not be opened. Try again or view diagnostics.";
  switch (error.code) {
    case "missing":
      return "No setup logs are available yet. Setup may have stopped before it created a log. Check engine status before retrying setup.";
    case "unavailable":
      return "Setup logs could not be opened safely. View diagnostics for the next step.";
    case "open-failed":
      return "Windows could not open the setup log folder. Try again or view diagnostics.";
    default:
      return "Setup logs could not be opened. Try again or view diagnostics.";
  }
}
