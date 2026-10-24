import { useState } from "react";
import { Box, ChevronDown, ChevronRight, Search } from "lucide-react";
import type * as backend from "../../backend";
import {
  navigation,
  navigationGroups,
  shortcutFor,
  type NavigationGroup,
  type Page,
} from "../../model/navigation";
import type { KubernetesState } from "../settings/app-settings-model";
import { canStartEngine, engineState } from "./engine-state";
import { useEngineLifecycle } from "./EngineLifecycle";

const storageKey = "orbwin.sidebar.collapsed";
function storedCollapse(): Partial<Record<NavigationGroup, boolean>> {
  try {
    const value = JSON.parse(window.localStorage.getItem(storageKey) ?? "{}");
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

export function Sidebar({
  page,
  onNavigate,
  runningContainers,
  kubernetes,
  onEnableKubernetes,
  onPalette,
  status,
  checking,
}: {
  page: Page;
  onNavigate: (page: Page) => void;
  runningContainers: number | null;
  kubernetes: KubernetesState;
  onEnableKubernetes: () => void;
  onPalette: () => void;
  status: backend.BackendStatus | null;
  checking: boolean;
}) {
  const lifecycle = useEngineLifecycle();
  const [collapsed, setCollapsed] = useState(storedCollapse);
  function toggle(group: NavigationGroup, value: boolean) {
    const next = { ...collapsed, [group]: value };
    setCollapsed(next);
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(next));
    } catch {
      /* Collapse state is a convenience; it still applies this session. */
    }
  }
  const engine = engineState(status);
  const connected = engine === "running";
  const startable = canStartEngine(status);
  const engineLabel = lifecycle.action
    ? lifecycle.stage || "Working…"
    : checking && !status
      ? "Checking engine…"
      : connected
        ? "Engine running"
        : engine === "offline"
          ? "Engine stopped"
          : "Engine unavailable";
  return (
    <aside className="sidebar">
      <div className="app-name">
        <Box size={18} aria-hidden="true" /> <strong>OrbWin</strong>
      </div>
      <button
        className="palette-trigger"
        onClick={onPalette}
        aria-keyshortcuts="Control+K"
      >
        <Search size={14} aria-hidden="true" />
        <span>Go to or run…</span>
        <kbd>Ctrl K</kbd>
      </button>
      <nav aria-label="Main">
        {navigationGroups.map((group) => {
          const items = navigation.filter((item) => item.group === group);
          const kubernetesOff =
            group === "Kubernetes" && kubernetes === "disabled";
          // Kubernetes defaults to collapsed while the cluster is reported off;
          // an explicit user choice always wins. The current page stays visible.
          const isCollapsed =
            (collapsed[group] ?? kubernetesOff) &&
            !items.some((item) => item.id === page);
          const listId = `nav-group-${group.toLowerCase()}`;
          return (
            <div key={group} className="nav-section">
              <h2 className="nav-group">
                <button
                  aria-expanded={!isCollapsed}
                  aria-controls={listId}
                  onClick={() => toggle(group, !isCollapsed)}
                >
                  {isCollapsed ? (
                    <ChevronRight size={12} aria-hidden="true" />
                  ) : (
                    <ChevronDown size={12} aria-hidden="true" />
                  )}
                  {group}
                  {kubernetesOff && <span className="nav-tag">Off</span>}
                </button>
              </h2>
              {kubernetesOff && (
                <div className="nav-cta">
                  <small>The optional cluster is disabled.</small>
                  <button className="button" onClick={onEnableKubernetes}>
                    Enable Kubernetes…
                  </button>
                </div>
              )}
              <div id={listId} hidden={isCollapsed}>
                {items.map(({ id, label, icon: Icon }) => {
                  const shortcut = shortcutFor(id);
                  return (
                    <button
                      key={id}
                      className={`nav-item ${page === id ? "selected" : ""}`}
                      aria-current={page === id ? "page" : undefined}
                      aria-keyshortcuts={shortcut
                        ?.replace("Ctrl", "Control")
                        .replace(",", "Comma")}
                      title={shortcut ? `${label} (${shortcut})` : label}
                      onClick={() => onNavigate(id)}
                    >
                      <Icon size={18} aria-hidden="true" />
                      <span>{label}</span>
                      {id === "containers" &&
                        runningContainers !== null &&
                        runningContainers > 0 && (
                          <span
                            className="nav-badge"
                            aria-label={`${runningContainers} running`}
                          >
                            {runningContainers}
                          </span>
                        )}
                    </button>
                  );
                })}
              </div>
            </div>
          );
        })}
      </nav>
      <div className="sidebar-engine">
        <span
          className={`status-dot ${connected ? "running" : ""} ${lifecycle.action ? "working" : ""}`}
          aria-hidden="true"
        />
        <div className="sidebar-engine-copy" role="status" aria-live="polite">
          <strong>{engineLabel}</strong>
          <small>
            {lifecycle.action
              ? "Cancel closes the client only"
              : status?.version
                ? `Docker ${status.version}`
                : "Engine not connected"}
          </small>
        </div>
        {lifecycle.action ? (
          <button className="button compact" onClick={lifecycle.cancel}>
            Cancel
          </button>
        ) : (
          <button
            role="switch"
            className="engine-switch"
            aria-checked={connected}
            aria-label="OrbWin engine"
            disabled={checking || (!connected && !startable)}
            title={
              connected
                ? "Stop the OrbWin engine"
                : startable
                  ? "Start the OrbWin engine"
                  : "Start is unavailable until the backend is verified and its engine is offline. Open Diagnostics."
            }
            onClick={() =>
              connected ? lifecycle.requestStop() : lifecycle.start()
            }
          >
            <span />
          </button>
        )}
      </div>
    </aside>
  );
}
