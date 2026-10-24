// Per-process Windows client selectors. Never install these as global defaults.
export const dockerEndpoint = "npipe:////./pipe/docker-rancher-desktop-orbwin";
// Testcontainers Node 12.2.1 uses WHATWG URL, which removes /./ segments.
// Keep native pipe separators for that client; verify the selected daemon ID.
export const nodeTestcontainersEndpoint = String.raw`npipe:////.\pipe\docker-rancher-desktop-orbwin`;
