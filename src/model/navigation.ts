import {
  Activity,
  Box,
  Boxes,
  Database,
  Globe,
  Hammer,
  Layers,
  Monitor,
  Network,
  Settings,
} from "lucide-react";
export const navigation = [
  { id: "containers", label: "Containers", icon: Boxes, group: "Docker" },
  { id: "volumes", label: "Volumes", icon: Database, group: "Docker" },
  { id: "images", label: "Images", icon: Layers, group: "Docker" },
  { id: "builds", label: "Builds", icon: Hammer, group: "Docker" },
  { id: "networks", label: "Networks", icon: Network, group: "Docker" },
  { id: "pods", label: "Pods", icon: Box, group: "Kubernetes" },
  { id: "services", label: "Services", icon: Globe, group: "Kubernetes" },
  { id: "deployments", label: "Deployments", icon: Boxes, group: "Kubernetes" },
  {
    id: "persistentVolumeClaims",
    label: "Storage claims",
    icon: Database,
    group: "Kubernetes",
  },
  { id: "wsl", label: "WSL distributions", icon: Monitor, group: "Linux" },
  { id: "settings", label: "Settings", icon: Settings, group: "General" },
  { id: "diagnostics", label: "Diagnostics", icon: Activity, group: "General" },
] as const;
export type Page = (typeof navigation)[number]["id"];
export type NavigationGroup = (typeof navigation)[number]["group"];
export const navigationGroups: NavigationGroup[] = [
  "Docker",
  "Kubernetes",
  "Linux",
  "General",
];
export type ResourcePage = "containers" | "images" | "volumes" | "networks";
export function isResourcePage(page: Page): page is ResourcePage {
  return ["containers", "images", "volumes", "networks"].includes(page);
}
export function isPage(value: unknown): value is Page {
  return navigation.some((item) => item.id === value);
}
// Ctrl+1…9 targets. An original Windows adaptation: OrbStack's shortcuts are
// not evidenced. Settings uses Ctrl+, separately.
export const shortcutPages: Page[] = [
  "containers",
  "volumes",
  "images",
  "builds",
  "networks",
  "pods",
  "services",
  "wsl",
  "diagnostics",
];
export function shortcutFor(page: Page) {
  if (page === "settings") return "Ctrl+,";
  const index = shortcutPages.indexOf(page);
  return index < 0 ? undefined : `Ctrl+${index + 1}`;
}
