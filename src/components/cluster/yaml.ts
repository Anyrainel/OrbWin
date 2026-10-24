// Minimal YAML renderer for objects that kubectl returned as JSON. Output is
// valid YAML; ambiguous scalars are emitted as JSON double-quoted strings, which
// YAML accepts. It is for display and copying only, never parsed back.
const RESERVED = /^(true|false|null|yes|no|on|off|y|n|~)$/i;
const PLAIN = /^[A-Za-z_./@][A-Za-z0-9 _./@:+=,()-]*$/;

function scalar(value: string): string {
  const unsafe =
    value === "" ||
    RESERVED.test(value) ||
    !PLAIN.test(value) ||
    /\s$/.test(value) ||
    value.includes(": ") ||
    value.includes(" #") ||
    !Number.isNaN(Number(value));
  return unsafe ? JSON.stringify(value) : value;
}
function isContainer(value: unknown): value is object {
  return typeof value === "object" && value !== null;
}
function isFilled(value: unknown): boolean {
  return Array.isArray(value)
    ? value.length > 0
    : isContainer(value) && Object.keys(value).length > 0;
}
function inline(value: unknown): string {
  if (value === null || value === undefined) return "null";
  if (Array.isArray(value)) return "[]";
  if (isContainer(value)) return "{}";
  if (typeof value === "string") return scalar(value);
  return String(value);
}
function lines(value: unknown, pad: string): string[] {
  const out: string[] = [];
  if (Array.isArray(value)) {
    for (const item of value) {
      if (isFilled(item)) {
        const sub = lines(item, pad + "  ");
        out.push(`${pad}- ${sub[0].slice(pad.length + 2)}`, ...sub.slice(1));
      } else out.push(`${pad}- ${inline(item)}`);
    }
  } else if (isContainer(value)) {
    for (const [key, child] of Object.entries(value)) {
      if (isFilled(child)) {
        out.push(`${pad}${scalar(key)}:`, ...lines(child, pad + "  "));
      } else out.push(`${pad}${scalar(key)}: ${inline(child)}`);
    }
  }
  return out;
}
export function toYaml(value: unknown): string {
  return isFilled(value) ? lines(value, "").join("\n") : inline(value);
}
