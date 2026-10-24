import {
  lazy,
  Suspense,
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
} from "react";
import { LocalPortLink } from "./LocalPortLink";
import {
  Box,
  ChevronDown,
  ChevronRight,
  Database,
  ExternalLink,
  Folder,
  Layers,
  MoreHorizontal,
  Network,
  Play,
  RotateCw,
  Square,
  Trash2,
} from "lucide-react";
import type * as backend from "../backend";
import type { ResourcePage } from "../model/navigation";
import { ContainerView, ResourceActions, useOperation } from "./Operations";
import { actionsFor, groupActions, type Action } from "./resources/actions";
import { ContextMenu, type MenuRequest } from "./resources/ContextMenu";
import { InfoPanel } from "./resources/InfoPanel";
import { StatsPanel } from "./resources/StatsPanel";
import {
  entries,
  imageUsers,
  isRunning,
  keyOf as identity,
  name,
  publishedPorts,
  publicationWarning,
  target as resourceTarget,
  text,
  volumeUsers,
  type Row,
  type SortMode,
} from "./resources/model";
import { loadLayout, saveLayout } from "./resources/persist";
import "../styles/resources.css";
const Terminal = lazy(() =>
  import("./Terminal").then((module) => ({ default: module.Terminal })),
);
export function Unavailable({
  busy,
  description,
  onDiagnostics,
}: {
  busy: boolean;
  description: string;
  onDiagnostics: () => void;
}) {
  return (
    <div className="empty-state">
      <Box size={30} />
      <h2>{busy ? "Connecting to engine…" : "Engine unavailable"}</h2>
      <p>{description}</p>
      <button className="button" onClick={onDiagnostics}>
        View diagnostics
      </button>
    </div>
  );
}
const tabs = ["Info", "Stats", "Logs", "Terminal", "Files"] as const;
const quickIcons: Record<string, typeof Play> = {
  start: Play,
  stop: Square,
  restart: RotateCw,
  remove: Trash2,
  run: Play,
};
const widths = { min: 260, max: 560 };
function clampWidth(width: number) {
  return Math.round(
    Math.max(
      widths.min,
      Math.min(width, widths.max, window.innerWidth - 190 - 320),
    ),
  );
}
export function ResourceList({
  rows,
  query,
  label,
  page,
  containers,
}: {
  rows: backend.Resource[];
  query: string;
  label: string;
  page: ResourcePage;
  // Complete container inventory for "in use" markers; omitted means unknown.
  containers?: backend.Resource[];
}) {
  const [layout] = useState(() => loadLayout(page));
  const [selected, setSelected] = useState<string[]>(
    layout.selected ? [layout.selected] : [],
  );
  const [primary, setPrimary] = useState<string | null>(
    layout.selected ?? null,
  );
  const [anchor, setAnchor] = useState<string | null>(primary);
  const [tab, setTab] = useState<(typeof tabs)[number]>("Info");
  const [collapsed, setCollapsed] = useState(
    () => new Set(layout.collapsed ?? []),
  );
  const [sort, setSort] = useState<SortMode>(layout.sort ?? "status");
  const [width, setWidth] = useState(layout.width);
  const [menu, setMenu] = useState<MenuRequest | null>(null);
  const open = useOperation();
  const refs = useRef(new Map<string, HTMLButtonElement>());
  const list = useRef<HTMLDivElement>(null);
  const inspector = useRef<HTMLElement>(null);
  const keyOf = (row: Row) => identity(page, row);
  const filtered = rows.filter((row) =>
    Object.values(row).some((value) =>
      text(value).toLowerCase().includes(query.toLowerCase()),
    ),
  );
  const listed = entries(page, filtered, sort);
  const visible = listed.flatMap((entry) =>
    entry.kind === "row"
      ? [entry.row]
      : collapsed.has(entry.project)
        ? []
        : entry.rows,
  );
  // Selection is derived from the current snapshot and filter; never act on an
  // object that disappeared after a refresh or silently select a different row.
  const chosen = new Set(selected);
  const selectedRows = filtered.filter((row) => chosen.has(keyOf(row)));
  const detail = filtered.find((row) => keyOf(row) === primary) ?? null;
  const Icon =
    page === "images"
      ? Layers
      : page === "volumes"
        ? Database
        : page === "networks"
          ? Network
          : Box;

  // The list pane belongs to the app shell; a user-chosen width is applied to
  // it inline and removed on unmount so the shell's responsive default returns.
  useLayoutEffect(() => {
    const pane = list.current?.parentElement;
    if (!pane?.classList.contains("list-pane") || !inspector.current) return;
    pane.style.width = width ? `${width}px` : "";
    inspector.current.style.left = width ? `${width}px` : "";
    return () => {
      pane.style.width = "";
    };
  }, [width]);

  function choose(keys: string[], focus: string | null, nextAnchor = focus) {
    setSelected(keys);
    setPrimary(focus);
    setAnchor(nextAnchor);
    saveLayout(page, { selected: focus ?? undefined });
  }
  function range(from: string | null, to: string) {
    const keys = visible.map(keyOf);
    const start = from ? keys.indexOf(from) : -1;
    const end = keys.indexOf(to);
    if (start < 0) return [to];
    return keys.slice(Math.min(start, end), Math.max(start, end) + 1);
  }
  function click(event: MouseEvent, row: Row) {
    const key = keyOf(row);
    if (event.shiftKey) {
      const span = range(anchor, key);
      choose(
        event.ctrlKey
          ? [...new Set([...selectedRows.map(keyOf), ...span])]
          : span,
        key,
        anchor,
      );
    } else if (event.ctrlKey || event.metaKey) {
      const current = selectedRows.map(keyOf);
      const next = chosen.has(key)
        ? current.filter((item) => item !== key)
        : [...current, key];
      choose(next, chosen.has(key) ? (next.at(-1) ?? null) : key, key);
    } else {
      if (key !== primary) setTab("Info");
      choose([key], key);
    }
  }
  function move(event: KeyboardEvent, row: Row) {
    const index = visible.indexOf(row);
    const key = keyOf(row);
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "a") {
      event.preventDefault();
      choose(visible.map(keyOf), key, anchor);
      return;
    }
    if (
      event.key === "ContextMenu" ||
      (event.shiftKey && event.key === "F10")
    ) {
      event.preventDefault();
      const box = event.currentTarget.getBoundingClientRect();
      showMenu(row, box.left + 24, box.bottom - 6);
      return;
    }
    if (event.key === "Delete") {
      event.preventDefault();
      const targets = chosen.has(key) ? selectedRows : [row];
      const remove = actionsFor(page, targets).find((a) => a.id === "remove");
      if (remove) run(remove);
      return;
    }
    if (event.key === "Escape" && selectedRows.length > 1) {
      // Narrow to the focused row; the app-level Escape (clear search) waits.
      event.stopPropagation();
      choose([key], key);
      return;
    }
    let next = index;
    if (event.key === "ArrowDown")
      next = Math.min(index + 1, visible.length - 1);
    else if (event.key === "ArrowUp") next = Math.max(index - 1, 0);
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = visible.length - 1;
    else return;
    event.preventDefault();
    const nextKey = keyOf(visible[next]);
    if (event.shiftKey)
      choose(range(anchor ?? key, nextKey), nextKey, anchor ?? key);
    else choose([nextKey], nextKey);
    refs.current.get(nextKey)?.focus();
  }
  function run(action: Action) {
    if (action.unavailable) return;
    if (action.copy)
      void navigator.clipboard?.writeText(action.copy).catch(() => {});
    else if (action.spec) open(action.spec);
  }
  function showMenu(row: Row, x: number, y: number) {
    const key = keyOf(row);
    // Acting on a row outside the selection selects it alone first.
    const targets = chosen.has(key) ? selectedRows : [row];
    if (!chosen.has(key)) choose([key], key);
    setMenu({
      x,
      y,
      label:
        targets.length > 1
          ? `Actions for ${targets.length} selected`
          : `Actions for ${name(row)}`,
      actions: actionsFor(page, targets),
      returnFocus: refs.current.get(key) ?? null,
    });
  }
  const menuRef = useRef(menu);
  menuRef.current = menu;
  const closeMenu = useCallback((restore: boolean) => {
    const focus = menuRef.current?.returnFocus;
    setMenu(null);
    if (restore) focus?.focus();
  }, []);
  function toggleGroup(group: string) {
    setCollapsed((previous) => {
      const next = new Set(previous);
      if (next.has(group)) next.delete(group);
      else next.add(group);
      saveLayout(page, { collapsed: [...next] });
      return next;
    });
  }
  function resize(next: number | undefined) {
    const value = next === undefined ? undefined : clampWidth(next);
    setWidth(value);
    saveLayout(page, { width: value });
  }

  function quickActions(row: Row) {
    const available = actionsFor(page, [row]).filter(
      (action) => quickIcons[action.id] && !action.unavailable,
    );
    const link = publishedPorts(row).find((port) => port.url);
    return (
      <span className="quick-actions">
        {page === "containers" && link?.url && isRunning(row) && (
          <LocalPortLink
            className="quick-action"
            tabIndex={-1}
            port={link.hostPort}
            aria-label={`Open localhost:${link.hostPort} for ${name(row)}`}
            title={`Open localhost:${link.hostPort}`}
          >
            <ExternalLink size={14} />
          </LocalPortLink>
        )}
        {available.map((action) => {
          const QuickIcon = quickIcons[action.id];
          return (
            <button
              key={action.id}
              className={`quick-action ${action.destructive ? "danger" : ""}`}
              tabIndex={-1}
              aria-label={`${action.label} ${name(row)}`}
              title={action.label.replace("…", "")}
              onClick={() => run(action)}
            >
              <QuickIcon size={14} />
            </button>
          );
        })}
        <button
          className="quick-action"
          tabIndex={-1}
          aria-label={`More actions for ${name(row)}`}
          title="More actions (Shift+F10)"
          onClick={(event) => {
            const box = event.currentTarget.getBoundingClientRect();
            showMenu(row, box.left, box.bottom);
          }}
        >
          <MoreHorizontal size={14} />
        </button>
      </span>
    );
  }
  function marker(row: Row) {
    if (!containers) return null;
    const used =
      page === "images"
        ? imageUsers(row, containers).length
        : page === "volumes"
          ? volumeUsers(String(row.Name), containers).length
          : 0;
    return used ? (
      <span className="in-use" title={`Used by ${used} listed containers`}>
        In use
      </span>
    ) : null;
  }
  function meta(row: Row) {
    if (page === "containers") return text(row.Status ?? "");
    if (page === "images")
      return [row.Size, row.CreatedSince].filter(Boolean).map(text).join(" · ");
    return text(row.Scope ?? "");
  }
  function renderRow(row: Row, grouped: boolean) {
    const key = keyOf(row);
    const isSelected = chosen.has(key);
    const stopped = page === "containers" && !isRunning(row);
    return (
      <div
        key={key}
        className={`resource-item ${stopped ? "stopped" : ""}`}
        onContextMenu={(event) => {
          event.preventDefault();
          showMenu(row, event.clientX, event.clientY);
        }}
      >
        <button
          ref={(element) => {
            if (element) refs.current.set(key, element);
            else refs.current.delete(key);
          }}
          className={`resource-row ${isSelected ? "selected" : ""} ${grouped ? "in-project" : ""}`}
          aria-pressed={isSelected}
          aria-describedby={`${page}-row-help`}
          onClick={(event) => click(event, row)}
          onKeyDown={(event) => move(event, row)}
        >
          <span className="resource-symbol">
            <Icon size={23} />
            {page === "containers" && (
              <i className={`status-dot ${stopped ? "" : "running"}`} />
            )}
          </span>
          <span className="row-copy">
            <strong>{name(row)}</strong>
            <small>{text(row.Image ?? row.Tag ?? row.Driver)}</small>
            {page === "containers" && publicationWarning(row) && (
              <small className="notice" title={publicationWarning(row)}>
                Host port unavailable
              </small>
            )}
          </span>
          <span className="row-meta">
            {marker(row)}
            {meta(row)}
          </span>
        </button>
        {quickActions(row)}
      </div>
    );
  }
  const batch = selectedRows.length > 1 ? actionsFor(page, selectedRows) : [];
  return (
    <>
      <div
        ref={list}
        className="resource-list"
        aria-label={`${label} inventory`}
      >
        <p id={`${page}-row-help`} className="visually-hidden">
          Ctrl or Shift click to select several. Shift+F10 opens actions; Delete
          asks to remove.
        </p>
        {page === "containers" && rows.length > 1 && (
          <label className="list-sort">
            Sort
            <select
              value={sort}
              onChange={(event) => {
                const value = event.target.value as SortMode;
                setSort(value);
                saveLayout(page, { sort: value });
              }}
            >
              <option value="status">Running first</option>
              <option value="name">Name</option>
            </select>
          </label>
        )}
        {batch.length > 0 && (
          <div
            className="batch-bar"
            role="toolbar"
            aria-label={`Actions for ${selectedRows.length} selected`}
          >
            <span aria-live="polite">{selectedRows.length} selected</span>
            {batch
              .filter((action) => !action.port && !action.copy)
              .map((action) => (
                <button
                  key={action.id}
                  className={`button ${action.destructive ? "danger" : ""}`}
                  disabled={!!action.unavailable}
                  title={action.unavailable}
                  onClick={() => run(action)}
                >
                  {action.label}
                </button>
              ))}
            <button
              className="button"
              onClick={() => choose(primary ? [primary] : [], primary)}
            >
              Clear
            </button>
          </div>
        )}
        {!filtered.length && (
          <div className="empty-state">
            <Icon size={30} />
            <h2>
              {rows.length
                ? "No matching results"
                : `No ${label.toLowerCase()} yet`}
            </h2>
            <p>
              {rows.length
                ? "Try another search or press Escape to clear it."
                : "The connected engine returned an empty inventory."}
            </p>
          </div>
        )}
        {listed.map((entry) => {
          if (entry.kind === "row") return renderRow(entry.row, false);
          const group = entry.project;
          const running = entry.rows.filter(isRunning).length;
          const actions = groupActions(group, entry.rows);
          const groupMenu = (x: number, y: number, focus: HTMLElement) =>
            setMenu({
              x,
              y,
              label: `Actions for project ${group}`,
              actions,
              returnFocus: focus,
            });
          return (
            <div key={`project:${group}`}>
              <div
                className={`resource-item project-item ${running ? "" : "stopped"}`}
                onContextMenu={(event) => {
                  event.preventDefault();
                  groupMenu(
                    event.clientX,
                    event.clientY,
                    event.currentTarget.querySelector("button")!,
                  );
                }}
              >
                <button
                  className="project-row"
                  aria-expanded={!collapsed.has(group)}
                  aria-describedby={`${page}-row-help`}
                  onClick={() => toggleGroup(group)}
                  onKeyDown={(event) => {
                    if (
                      event.key === "ContextMenu" ||
                      (event.shiftKey && event.key === "F10")
                    ) {
                      event.preventDefault();
                      const box = event.currentTarget.getBoundingClientRect();
                      groupMenu(
                        box.left + 24,
                        box.bottom - 6,
                        event.currentTarget,
                      );
                    }
                  }}
                >
                  {collapsed.has(group) ? (
                    <ChevronRight size={14} />
                  ) : (
                    <ChevronDown size={14} />
                  )}
                  <Layers size={21} />
                  <strong>{group}</strong>
                  <small>
                    {running}/{entry.rows.length} running
                  </small>
                </button>
                <span className="quick-actions">
                  {actions
                    .filter(
                      (action) =>
                        ["start", "stop", "restart"].includes(action.id) &&
                        !action.unavailable,
                    )
                    .map((action) => {
                      const QuickIcon = quickIcons[action.id];
                      return (
                        <button
                          key={action.id}
                          className="quick-action"
                          tabIndex={-1}
                          aria-label={`${action.label} project ${group}`}
                          title={`${action.label} project`}
                          onClick={() => run(action)}
                        >
                          <QuickIcon size={14} />
                        </button>
                      );
                    })}
                  <button
                    className="quick-action"
                    tabIndex={-1}
                    aria-label={`More actions for project ${group}`}
                    title="More actions (Shift+F10)"
                    onClick={(event) => {
                      const box = event.currentTarget.getBoundingClientRect();
                      groupMenu(box.left, box.bottom, event.currentTarget);
                    }}
                  >
                    <MoreHorizontal size={14} />
                  </button>
                </span>
              </div>
              {!collapsed.has(group) &&
                entry.rows.map((row) => renderRow(row, true))}
            </div>
          );
        })}
      </div>
      {menu && <ContextMenu menu={menu} onRun={run} onClose={closeMenu} />}
      <aside
        ref={inspector}
        className="inspector"
        aria-label="Resource details"
      >
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize list"
          aria-valuemin={widths.min}
          aria-valuemax={widths.max}
          aria-valuenow={
            width ?? list.current?.parentElement?.offsetWidth ?? widths.min
          }
          tabIndex={0}
          className="list-resizer"
          title="Drag to resize; double-click to reset"
          onDoubleClick={() => resize(undefined)}
          onKeyDown={(event) => {
            const current =
              width ?? list.current?.parentElement?.offsetWidth ?? 350;
            if (event.key === "ArrowLeft") resize(current - 16);
            else if (event.key === "ArrowRight") resize(current + 16);
            else if (event.key === "Home" || event.key === "Enter")
              resize(undefined);
            else return;
            event.preventDefault();
          }}
          onPointerDown={(event) => {
            const start = event.clientX;
            const initial =
              list.current?.parentElement?.getBoundingClientRect().width ?? 350;
            const element = event.currentTarget;
            element.setPointerCapture(event.pointerId);
            const drag = (next: PointerEvent) =>
              setWidth(clampWidth(initial + next.clientX - start));
            const stop = (next: PointerEvent) => {
              element.removeEventListener("pointermove", drag);
              element.removeEventListener("pointerup", stop);
              element.removeEventListener("pointercancel", stop);
              resize(initial + next.clientX - start);
            };
            element.addEventListener("pointermove", drag);
            element.addEventListener("pointerup", stop);
            element.addEventListener("pointercancel", stop);
          }}
        />
        <header className="inspector-toolbar">
          <div className="tabs" role="tablist" aria-label="Resource views">
            {tabs.map((item, index) => (
              <button
                key={item}
                id={`tab-${item}`}
                role="tab"
                aria-selected={tab === item}
                aria-controls="resource-detail"
                tabIndex={tab === item ? 0 : -1}
                onClick={() => setTab(item)}
                onKeyDown={(event) => {
                  if (event.key !== "ArrowLeft" && event.key !== "ArrowRight")
                    return;
                  event.preventDefault();
                  const next =
                    tabs[
                      (index +
                        (event.key === "ArrowRight" ? 1 : tabs.length - 1)) %
                        tabs.length
                    ];
                  setTab(next);
                  document.getElementById(`tab-${next}`)?.focus();
                }}
              >
                {item}
              </button>
            ))}
          </div>
        </header>
        <div
          id="resource-detail"
          role="tabpanel"
          aria-labelledby={`tab-${tab}`}
          className="detail-content"
        >
          {!detail ? (
            <div className="inspector-empty">
              <Icon size={38} />
              <h2>
                Select{" "}
                {page === "images"
                  ? "an image"
                  : page === "volumes"
                    ? "a volume"
                    : page === "networks"
                      ? "a network"
                      : "a container"}
              </h2>
              <p>Choose an item to inspect its details.</p>
            </div>
          ) : (
            <>
              <div className="detail-title">
                <Icon size={30} />
                <div>
                  <h2>{name(detail)}</h2>
                  <p>{text(detail.Status ?? detail.Tag ?? detail.Driver)}</p>
                </div>
              </div>
              <ResourceActions page={page} row={detail} />
              {tab === "Info" ? (
                <>
                  <InfoPanel
                    key={keyOf(detail)}
                    page={page}
                    row={detail}
                    containers={containers}
                  />
                  <div className="capability-note">
                    <Folder size={16} />
                    <span>
                      File browsing is not connected in this development build.
                    </span>
                  </div>
                </>
              ) : page === "containers" && tab === "Stats" ? (
                <StatsPanel
                  key={resourceTarget(page, detail)}
                  target={resourceTarget(page, detail)}
                />
              ) : page === "containers" && tab === "Terminal" ? (
                <Suspense fallback={<p role="status">Loading terminal…</p>}>
                  <Terminal
                    key={resourceTarget(page, detail)}
                    target={resourceTarget(page, detail)}
                  />
                </Suspense>
              ) : page === "containers" && tab === "Logs" ? (
                <ContainerView
                  key={`${resourceTarget(page, detail)}:${tab}`}
                  target={resourceTarget(page, detail)}
                  tab={tab}
                />
              ) : (
                <div className="blocked-view">
                  <h3>{tab} unavailable</h3>
                  <p>
                    {tab === "Stats"
                      ? "Live resource statistics apply to containers only."
                      : tab === "Logs"
                        ? "Logs apply to containers only."
                        : tab === "Terminal"
                          ? "Interactive sessions apply to containers only."
                          : "Native filesystem access is not connected. Image contents must remain read-only."}
                  </p>
                  <p>The selected resource has not been changed.</p>
                </div>
              )}
            </>
          )}
        </div>
      </aside>
    </>
  );
}
