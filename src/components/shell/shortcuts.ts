// Global shortcuts must never steal keys from text entry or the terminal
// (terminal/text editing must not trigger app commands; see
// docs/verification/orbstack-ui-flows-2026-10-23.md, deletion/terminal row).
export type ShortcutTarget = {
  tagName?: string;
  isContentEditable?: boolean;
  closest?: (selector: string) => unknown;
} | null;

export function isTerminal(target: ShortcutTarget) {
  return Boolean(target?.closest?.(".xterm, .terminal-host"));
}
export function isTextEntry(target: ShortcutTarget) {
  const tag = target?.tagName?.toUpperCase();
  return (
    tag === "INPUT" ||
    tag === "TEXTAREA" ||
    tag === "SELECT" ||
    Boolean(target?.isContentEditable) ||
    isTerminal(target)
  );
}

export type ShortcutKey = {
  key: string;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  metaKey: boolean;
};
export type ShortcutCommand =
  | { kind: "palette" }
  | { kind: "settings" }
  | { kind: "page"; index: number };

// Returns the app command for a key event, or null. Terminals keep every key;
// text fields keep everything except the palette (Ctrl+K has no editing
// meaning in a plain Windows text field).
export function shortcutCommand(
  event: ShortcutKey,
  target: ShortcutTarget,
): ShortcutCommand | null {
  if (!event.ctrlKey || event.altKey || event.metaKey || event.shiftKey)
    return null;
  if (isTerminal(target)) return null;
  const key = event.key.toLowerCase();
  if (key === "k") return { kind: "palette" };
  if (isTextEntry(target)) return null;
  if (key === ",") return { kind: "settings" };
  if (/^[1-9]$/.test(key)) return { kind: "page", index: Number(key) - 1 };
  return null;
}
