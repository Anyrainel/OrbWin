import type { Page, SortMode } from "./model";

// Per-page layout preferences. Never authoritative: a stored selection only
// applies while that identity is still in the current snapshot.
export type Layout = {
  collapsed?: string[];
  width?: number;
  sort?: SortMode;
  selected?: string;
};
const key = (page: Page) => `orbwin.resources.${page}`;
export function loadLayout(page: Page): Layout {
  try {
    const value = JSON.parse(localStorage.getItem(key(page)) ?? "{}");
    if (!value || typeof value !== "object" || Array.isArray(value)) return {};
    const result: Layout = {};
    if (Array.isArray(value.collapsed))
      result.collapsed = value.collapsed
        .filter(
          (item: unknown) => typeof item === "string" && item.length <= 256,
        )
        .slice(0, 512);
    if (typeof value.width === "number" && Number.isFinite(value.width))
      result.width = Math.max(240, Math.min(700, value.width));
    if (value.sort === "status" || value.sort === "name")
      result.sort = value.sort;
    if (typeof value.selected === "string" && value.selected.length <= 256)
      result.selected = value.selected;
    return result;
  } catch {
    return {};
  }
}
export function saveLayout(page: Page, patch: Layout) {
  try {
    localStorage.setItem(
      key(page),
      JSON.stringify({ ...loadLayout(page), ...patch }),
    );
  } catch {
    /* Storage can be unavailable; layout then lasts for this session only. */
  }
}
