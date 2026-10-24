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
    return value && typeof value === "object" ? value : {};
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
