export function ClientGuidance() {
  return (
    <>
      <dt>OrbWin CLI</dt>
      <dd>
        <p>
          The packaged orbwin-cli.exe and orbwin-docker.exe live beside the
          desktop application. Set $orbwinInstall to your installation directory
          in PowerShell, then run:
        </p>
        <pre className="settings-code">{`& (Join-Path $orbwinInstall 'orbwin-cli.exe') status
& (Join-Path $orbwinInstall 'orbwin-cli.exe') docker ps
. (Join-Path $orbwinInstall 'completions/OrbWin.ps1')`}</pre>
        <p>
          Completion applies to this PowerShell session. No PATH, profile or
          Docker context changes are needed. Start and stop commands affect
          OrbWin workloads; status only reads the current engine.
        </p>
      </dd>
      <dt>VS Code Dev Containers</dt>
      <dd>
        <p>
          To choose OrbWin, set Dev Containers: Docker Path
          (dev.containers.dockerPath) to the absolute path of orbwin-docker.exe.
          This is an application setting, so review its effect on other Dev
          Containers projects. A workspace setting cannot isolate it to one
          project. Restore your previous setting when selecting another engine.
        </p>
        <p>
          For an explicit command invocation, the Dev Containers CLI accepts
          --docker-path followed by that executable path. The wrapper targets
          OrbWin and uses its saved registry credentials. Selecting it permits
          launched Docker, Compose and supported Buildx commands to request
          those credentials. Sign in through Registries first when needed;
          credentials are not copied into your normal Docker configuration.
        </p>
        <p>
          Windows and registered WSL folder open, file writes and reopen were
          tested with Dev Containers 0.469.0. Compose builds and private
          registry pulls still require qualification. Custom remote builders are
          unavailable through this wrapper.
        </p>
      </dd>
    </>
  );
}
