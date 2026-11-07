import { wslMigrationDraft } from "../../model/migration-wsl-draft";
import { migrationDraft } from "../../model/migration-draft";
import { useEffect, useMemo, useRef, useState } from "react";
import * as backend from "../../backend";
import {
  migrationImageExport,
  migrationVolumeExport,
  parseMigrationImages,
  parseMigrationWSLSources,
  parseMigrationVolumeReview,
  type MigrationImageReview,
  type MigrationVolumeReview,
} from "../../model/migration-images";
import { composeSpec, wslComposeSpec, useOperation } from "../Operations";
import { migrationProjectReview } from "../../model/migration-projects";
import { MigrationWslProject } from "./MigrationWslProject";
import { MigrationVolumeHistory } from "./MigrationVolumeHistory";
import { MigrationImageHistory } from "./MigrationImageHistory";
import { MigrationMountMapping } from "./MigrationMountMapping";
import { MigrationProjectImages } from "./MigrationProjectImages";
import { MigrationProjectAccounting } from "./MigrationProjectAccounting";
import {
  migrationMountDraft,
  reviewMigrationMounts,
} from "../../model/migration-mounts";
import type { migrationWslReview } from "../../model/migration-wsl";
import { containDialogTab } from "../shell/dialog-focus";

export function MigrationImages({ connected }: { connected: boolean }) {
  return <MigrationExports connected={connected} resource="images" />;
}
export function MigrationVolumes({ connected }: { connected: boolean }) {
  return <MigrationExports connected={connected} resource="volumes" />;
}
export function MigrationExports({
  connected,
  resource,
  targetProjects = [],
  targetVolumes = [],
}: {
  connected: boolean;
  resource: "images" | "volumes" | "projects";
  targetProjects?: string[];
  targetVolumes?: string[];
}) {
  const volumes = resource === "volumes";
  const projects = resource === "projects";
  const open = useOperation();
  const [mappingFile, setMappingFile] = useState<{
    revision: string;
    path: string;
    contents: string;
    sha256: string;
  } | null>(null);
  const [mappingSelection, setMappingSelection] = useState<{
    revision: string;
    mapping: Record<string, string>;
  } | null>(null);
  const [visible, setVisible] = useState(false),
    [busy, setBusy] = useState(false);
  const [review, setReview] = useState<MigrationImageReview | null>(null),
    [error, setError] = useState("");
  const [imageID, setImageID] = useState(""),
    [reference, setReference] = useState("");
  const [volume, setVolume] = useState("");
  const [sourceProject, setSourceProject] = useState("");
  const [volumeReview, setVolumeReview] =
    useState<MigrationVolumeReview | null>(null);
  const [sourceKind, setSourceKind] = useState<
    "docker-desktop" | "rancher-desktop"
  >("docker-desktop");
  const [registrations, setRegistrations] = useState<
    { id: string; name: string }[] | null
  >(null);
  const [registration, setRegistration] = useState("");
  const rancher = sourceKind === "rancher-desktop";
  const sourceName = rancher ? "Rancher Desktop" : "Docker Desktop";
  const dialog = useRef<HTMLDialogElement>(null),
    trigger = useRef<HTMLButtonElement>(null);
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  useEffect(() => {
    if (visible && dialog.current && !dialog.current.open) {
      dialog.current.showModal();
      return () => trigger.current?.focus();
    }
  }, [visible]);
  function close() {
    pending.current?.abort();
    pending.current = null;
    setBusy(false);
    setVisible(false);
    setReview(null);
    setImageID("");
    setReference("");
    setVolume("");
    setVolumeReview(null);
    setSourceProject("");
    setRegistrations(null);
    setRegistration("");
  }
  async function readSource(
    request: backend.Operation,
    accept: (text: string) => void,
  ) {
    pending.current?.abort();
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    let output = "",
      diagnostic = "";
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        request,
        (text, stream) => {
          if (stream === "stderr")
            diagnostic = (diagnostic + text).slice(-8192);
          else {
            output += text;
            if (output.length > 2 * 1024 * 1024) abort.abort();
          }
        },
        abort.signal,
      );
      if (pending.current !== abort) return;
      if (abort.signal.aborted)
        throw new Error(
          "Source read cancelled or exceeded the desktop preview budget. No source data was changed.",
        );
      if (!result.success) throw new Error(diagnostic || result.message);
      accept(output);
    } catch (error) {
      if (pending.current === abort) setError(String(error));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  function inspect() {
    setSourceProject("");
    const request: backend.Operation = rancher
      ? {
          kind: "reviewRancherMigration",
          registration,
          machine: review?.source.wsl?.machine ?? "",
        }
      : { kind: "reviewDesktopMigration" };
    if (rancher && !registration) return;
    setReview(null);
    setImageID("");
    setReference("");
    setVolume("");
    setVolumeReview(null);
    void readSource(request, (text) => {
      const value = parseMigrationImages(text);
      if (
        value.source.kind !== sourceKind ||
        (rancher && value.source.wsl?.registration !== registration)
      )
        throw new Error("Source choice changed; inspect it again.");
      setReview(value);
    });
  }
  function chooseSource(kind: "docker-desktop" | "rancher-desktop") {
    setSourceProject("");
    pending.current?.abort();
    pending.current = null;
    setBusy(false);
    setError("");
    setSourceKind(kind);
    setReview(null);
    setImageID("");
    setReference("");
    setRegistrations(null);
    setRegistration("");
    setVolume("");
    setVolumeReview(null);
  }
  function listRegistrations() {
    setSourceProject("");
    setReview(null);
    setImageID("");
    setReference("");
    setRegistration("");
    setRegistrations(null);
    setVolume("");
    setVolumeReview(null);
    void readSource({ kind: "migrationWslSources" }, (text) =>
      setRegistrations(parseMigrationWSLSources(text)),
    );
  }
  function checkVolume() {
    if (!review || !volume || busy) return;
    const wsl = review.source.wsl;
    setVolumeReview(null);
    void readSource(
      rancher && wsl
        ? {
            kind: "reviewRancherMigrationVolume",
            source: review.source.daemon,
            registration: wsl.registration,
            machine: wsl.machine,
            volume,
          }
        : {
            kind: "reviewDesktopMigrationVolume",
            source: review.source.daemon,
            volume,
          },
      (text) =>
        setVolumeReview(parseMigrationVolumeReview(text, review, volume)),
    );
  }
  const image = review?.images.find((image) => image.id === imageID);
  const projectReview = useMemo(
    () =>
      review &&
      sourceProject &&
      review.projectMembers.some((member) => member.project === sourceProject)
        ? {
            ...migrationProjectReview(review, sourceProject),
            revision: crypto.randomUUID(),
          }
        : null,
    [review, sourceProject],
  );
  const revision = projectReview?.revision ?? "";
  const mapping =
    mappingSelection?.revision === revision ? mappingSelection.mapping : {};
  const namespace = revision.replaceAll("-", "").slice(0, 16);
  let mappingDraft = "";
  try {
    if (projectReview)
      mappingDraft = migrationMountDraft(
        reviewMigrationMounts(projectReview.members),
        mapping,
        namespace,
        targetVolumes,
      );
  } catch {
    /* The reviewer displays the precise blocking selection. */
  }
  const currentMappingFile =
    mappingFile?.revision === revision && mappingFile.contents === mappingDraft
      ? mappingFile
      : null;
  function continueWsl(prepared: ReturnType<typeof migrationWslReview>) {
    if (!projectReview || busy || !prepared.inputs.connection) return;
    let provenance;
    try {
      provenance = wslMigrationDraft(
        projectReview,
        {
          name: prepared.inputs.distribution,
          registration: prepared.inputs.connection.registration!,
          source: prepared.inputs.connection.source,
          signature: prepared.inputs.connection.signature,
        },
        prepared.mappingFile
          ? { namespace, selections: mapping, file: prepared.mappingFile }
          : undefined,
      );
    } catch (error) {
      setError(String(error));
      return;
    }
    const spec = wslComposeSpec(
      prepared.inputs.distribution,
      prepared.inputs.connection.source,
      "validate",
      { ...prepared.inputs, migration: provenance },
    );
    if (projectReview.explicitEnvFiles)
      spec.fields = spec.fields?.map((field) =>
        field.key === "envFiles"
          ? {
              ...field,
              required: true,
              label:
                "Original interpolation env files in order (required for this source review; one Linux project path per line)",
            }
          : field,
      );
    spec.reviewDetails = `Source ${sourceName}, daemon ${projectReview.source.daemon}, project ${projectReview.inputs.project}. ${prepared.message} Source volumes, image transfer and destination configuration mapping are separate. Review project-name/port collisions and coordinate shared host writers before any separately reviewed Up. ${spec.reviewDetails}`;
    close();
    open({
      ...spec,
      confirmLabel: "Validate Linux files",
      returnFocus: trigger.current ?? undefined,
    });
  }
  function continueExport() {
    if (!review || busy) return;
    if (projects) {
      if (!projectReview || (mappingDraft && !currentMappingFile)) return;
      let provenance;
      try {
        provenance = migrationDraft(
          projectReview,
          currentMappingFile
            ? {
                namespace,
                selections: mapping,
                file: {
                  path: currentMappingFile.path,
                  sha256: currentMappingFile.sha256,
                },
              }
            : undefined,
        );
      } catch (error) {
        setError(String(error));
        return;
      }
      const spec = composeSpec(
        "validate",
        {
          ...projectReview.inputs,
          migration: provenance,
          overrides: currentMappingFile
            ? [...projectReview.inputs.overrides, currentMappingFile.path]
            : projectReview.inputs.overrides,
        },
        `Source ${sourceName}, daemon ${projectReview.source.daemon}, project ${sourceProject}. Reuses selected files on this computer; no source workloads or data are moved. ${projectReview.message} Source named volumes require separate export/restore and explicit configuration mapping before startup. Host bind files remain shared; coordinate writers. Profiles, env files and secrets must be reviewed separately. ${targetProjects.includes(sourceProject) ? "OrbWin already has members with this project name; choose another name for a separate destination." : "No OrbWin members with this project name were observed; this is not a reservation."} Validation does not start the destination. After validation you can remember these inputs and reopen them from Compose project controls for separately reviewed Up.`,
      );
      if (projectReview.explicitEnvFiles)
        spec.fields = spec.fields?.map((field) =>
          field.key === "envFiles"
            ? {
                ...field,
                required: true,
                label:
                  "Original interpolation env files in order (required for this source review; one absolute Windows path per line)",
              }
            : field,
        );
      close();
      open({
        ...spec,
        confirmLabel: "Validate files",
        returnFocus: trigger.current ?? undefined,
      });
      return;
    }
    if (volumes) {
      if (!volumeReview || volumeReview.volume !== volume) return;
      const request = migrationVolumeExport(volumeReview);
      const selected = volumeReview.volume;
      close();
      open({
        title: `Export selected ${rancher ? "Rancher" : "Desktop"} volume`,
        confirmLabel: "Export volume",
        returnFocus: trigger.current ?? undefined,
        request,
        fields: [
          {
            key: "destination",
            label: "New portable package on a local Windows drive",
            save: true,
          },
        ],
        description: `Reads ${selected} from ${sourceName}, source daemon ${review.source.daemon}${review.source.wsl ? ` through ${review.source.wsl.name} (registration ${review.source.wsl.registration}, machine ${review.source.wsl.machine})` : ""}. Keep writable users stopped until export finishes; no workload is stopped automatically. Uses a temporary networkless read-only helper and bundled scratch image on the source, with exact ownership and cleanup. Source data is retained. Allow roughly twice the payload in host staging plus headroom; existing files are never replaced. Unsupported extended attributes, ACLs and special objects are refused. This is a quiesced file copy, not an application/database backup. Cancellation can leave tools and partial or unconfirmed completed packages; retain the printed transaction receipt and use Recover source export tools. After export, restore the verified package to a new OrbWin volume. Project files, credentials, settings and Kubernetes state are separate.`,
      });
      return;
    }
    if (!image || !reference) return;
    exportImage(image.id, reference);
  }
  function exportImage(selectedID: string, selectedReference: string) {
    if (!review || busy) return;
    const request = migrationImageExport(review, selectedID, selectedReference);
    const source = `${review.source.product || sourceName}, source daemon ${review.source.daemon}`;
    if (projects) setVisible(false);
    else close();
    open({
      title: `Export selected ${rancher ? "Rancher" : "Desktop"} image`,
      confirmLabel: "Export image",
      returnFocus: trigger.current ?? undefined,
      request,
      fields: [
        {
          key: "destination",
          label: "New .tar archive on a local Windows drive",
          save: true,
        },
      ],
      description: `Reads ${selectedReference} (${selectedID}) from ${source}${review.source.wsl ? ` through ${review.source.wsl.name} (registration ${review.source.wsl.registration}, machine ${review.source.wsl.machine})` : ""}. Source images, tags and workloads are retained. Exports the complete image, including its embedded data and environment configuration. Requires space for a full Docker archive; no existing file is replaced. Cancellation can leave a named partial or an unconfirmed completed archive. After export, use Import image archive to review its checksum, platform and destination tag conflicts before loading it into OrbWin. Project files, volumes, host registry credentials/settings and Kubernetes state are separate.`,
    });
  }
  return (
    <section
      className="image-import-controls"
      aria-label={`Selected-source ${projects ? "project review" : volumes ? "volume" : "image"} transfer`}
    >
      <div className="action-row">
        <button
          ref={trigger}
          className="button"
          disabled={!connected}
          onClick={() => {
            setError("");
            setVisible(true);
          }}
        >
          {projects
            ? "Review project from another engine…"
            : "Export from another engine…"}
        </button>
        {volumes && (
          <button
            className="button"
            onClick={() =>
              open({
                title: "Recover source export tools",
                confirmLabel: "Recover tools",
                destructive: true,
                request: {
                  kind: "recoverDesktopMigrationVolume",
                  transaction: "",
                },
                fields: [
                  {
                    key: "transaction",
                    label: "Recorded source export transaction ID",
                  },
                ],
                description:
                  "Uses the exact inactive receipt to verify the recorded Desktop or Rancher source and remove only that job's temporary helper and untagged scratch image. Active, changed or adopted resources are retained. Source volumes and host packages are never deleted; recovery does not resume export or infer whether an archive was published. Retain the receipt and inspect the selected destination before retrying.",
              })
            }
          >
            Recover source export tools…
          </button>
        )}
      </div>
      {volumes && <MigrationVolumeHistory />}
      {!volumes && <MigrationImageHistory />}
      {visible && (
        <dialog
          ref={dialog}
          className="operation-dialog image-import-dialog"
          aria-labelledby={`migration-${resource}-heading`}
          onCancel={(event) => {
            event.preventDefault();
            close();
          }}
          onKeyDown={containDialogTab}
        >
          <h2 id={`migration-${resource}-heading`}>
            {projects
              ? "Review a source project"
              : volumes
                ? "Transfer a volume"
                : "Transfer an image"}
          </h2>
          {
            <label>
              Source engine
              <select
                disabled={busy}
                value={sourceKind}
                onChange={(event) =>
                  chooseSource(
                    event.target.value as "docker-desktop" | "rancher-desktop",
                  )
                }
              >
                <option value="docker-desktop">Docker Desktop</option>
                <option value="rancher-desktop">
                  Rancher Desktop through WSL
                </option>
              </select>
            </label>
          }
          {rancher ? (
            <>
              <p>
                Rancher access uses Linux root in a selected ordinary Ubuntu
                24.04 WSL distro. Inspection may start that distro; Rancher
                itself must already be running. Uses a temporary verified
                connection tool and does not change global Docker defaults.
                Current source Engine support: 29.5.3 on Linux/amd64.
              </p>
              <button
                className="button"
                disabled={busy}
                onClick={listRegistrations}
              >
                {registrations ? "Refresh WSL choices" : "List WSL connections"}
              </button>
              {registrations && (
                <label>
                  WSL connection
                  <select
                    disabled={busy}
                    value={registration}
                    onChange={(event) => {
                      setRegistration(event.target.value);
                      setSourceProject("");
                      setReview(null);
                      setImageID("");
                      setReference("");
                      setVolume("");
                      setVolumeReview(null);
                      setError("");
                    }}
                  >
                    <option value="">Choose an ordinary WSL distro</option>
                    {registrations.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name} · {item.id}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {registrations?.length === 0 && (
                <p>
                  No eligible ordinary WSL 2 registrations. Engine-owned
                  distributions cannot be selected.
                </p>
              )}
            </>
          ) : (
            <p>
              Reads the dedicated Docker Desktop Linux endpoint. Start Desktop
              yourself if it is unavailable. This adapter currently qualifies
              Engine 29.8.0 on Linux/amd64; it never changes global defaults or
              starts an engine.
            </p>
          )}
          <p>
            {!projects && (
              <>
                Export and reviewed OrbWin {volumes ? "restore" : "import"} are
                separate steps.{" "}
              </>
            )}
            {projects
              ? "Project files are reused after review; data and settings are not copied automatically."
              : "Project files can be reviewed from Compose project controls."}
          </p>
          <div className="action-row">
            <button
              className="button"
              disabled={busy || !connected || (rancher && !registration)}
              onClick={() => void inspect()}
            >
              {busy
                ? "Reading source…"
                : review
                  ? "Refresh source"
                  : `Inspect ${rancher ? "Rancher" : "Desktop"} source`}
            </button>
            {busy && (
              <button
                className="button"
                onClick={() => pending.current?.abort()}
              >
                Cancel source read
              </button>
            )}
          </div>
          {error && <p role="alert">{error}</p>}
          {review && (
            <>
              <p role="status">
                {review.source.product || sourceName} · Engine{" "}
                {review.source.engineVersion}. {review.images.length} images,{" "}
                {review.volumes.length} volumes, {review.projectMembers.length}{" "}
                Compose members observed.
              </p>
              <p>
                Source: <code>{review.source.daemon}</code>
              </p>
              {review.source.wsl && (
                <p>
                  Verified WSL machine: <code>{review.source.wsl.machine}</code>
                </p>
              )}
              {!volumes && !projects && (
                <label>
                  Source image
                  <select
                    value={imageID}
                    onChange={(event) => {
                      const next = review.images.find(
                        (image) => image.id === event.target.value,
                      );
                      setImageID(next?.id ?? "");
                      setReference(
                        next?.tags.find((tag) => tag !== "<none>:<none>") ??
                          next?.id ??
                          "",
                      );
                    }}
                  >
                    <option value="">Choose an image</option>
                    {review.images.map((image) => (
                      <option key={image.id} value={image.id}>
                        {image.tags
                          .filter((tag) => tag !== "<none>:<none>")
                          .join(", ") || "Untagged image"}{" "}
                        · {image.id.slice(7, 19)}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {!volumes && !projects && image && (
                <label>
                  Export reference
                  <select
                    value={reference}
                    onChange={(event) => setReference(event.target.value)}
                  >
                    {image.tags
                      .filter((tag) => tag !== "<none>:<none>")
                      .map((tag) => (
                        <option key={tag} value={tag}>
                          {tag}
                        </option>
                      ))}
                    <option value={image.id}>
                      Immutable ID (original tags may be absent from archive)
                    </option>
                  </select>
                </label>
              )}
              {!volumes && !projects && review.images.length === 0 && (
                <p>No source images to export.</p>
              )}
              {projects && (
                <>
                  <label>
                    Source project
                    <select
                      value={sourceProject}
                      disabled={busy}
                      onChange={(event) => setSourceProject(event.target.value)}
                    >
                      <option value="">Choose a source project</option>
                      {[
                        ...new Set(
                          review.projectMembers.map((member) => member.project),
                        ),
                      ]
                        .sort()
                        .map((project) => (
                          <option key={project} value={project}>
                            {project}
                          </option>
                        ))}
                    </select>
                  </label>
                  {review.projectMembers.length === 0 && (
                    <p>
                      No Compose-labelled source members were observed. Projects
                      without containers cannot be discovered; choose their
                      files through Compose project controls.
                    </p>
                  )}
                  {projectReview && (
                    <>
                      <p role="status">{projectReview.message}</p>
                      <MigrationProjectImages
                        key={`images:${projectReview.revision}`}
                        review={projectReview}
                        busy={busy}
                        onExport={exportImage}
                      />
                      <MigrationProjectAccounting
                        key={`accounting:${projectReview.revision}`}
                        project={projectReview}
                        mapping={mapping}
                        busy={busy}
                      />
                      <p>
                        Observed ordinary services:{" "}
                        {projectReview.services.join(", ") || "none"}. These do
                        not establish the enabled profiles.
                      </p>
                      <p>
                        Source volumes:{" "}
                        {projectReview.volumes.join(", ") || "none observed"}.
                        Export/restore and destination Compose mapping are
                        separate steps.
                      </p>
                      {projectReview.bindMounts && (
                        <p>
                          Host bind mounts are present. Reusing files can share
                          writable host data with the source; coordinate writers
                          before destination startup.
                        </p>
                      )}
                      {projectReview.explicitEnvFiles && (
                        <p>
                          Explicit source env files were indicated. Their paths
                          and contents were not transferred; review the
                          configuration manually.
                        </p>
                      )}
                      {targetProjects.includes(sourceProject) && (
                        <p role="status">
                          OrbWin already has this project name. Choose a
                          different name to create an independent destination.
                        </p>
                      )}
                      <details>
                        <summary>
                          Observed source paths (untrusted labels)
                        </summary>
                        {projectReview.members.map((member) => (
                          <div key={member.container}>
                            <p>
                              {member.service || "Unknown service"}
                              {member.oneOff ? " (one-off)" : ""} ·{" "}
                              {member.container.slice(0, 12)}
                            </p>
                            <p>
                              Working directory:{" "}
                              <code>
                                {member.workingDirectory || "not recorded"}
                              </code>
                            </p>
                            <p>
                              File label:{" "}
                              <code>
                                {member.configFilesLabel || "not recorded"}
                              </code>
                            </p>
                          </div>
                        ))}
                      </details>
                      <MigrationWslProject
                        key={`wsl:${projectReview.revision}`}
                        review={projectReview}
                        onReview={continueWsl}
                        mappingContents={mappingDraft}
                      />
                      <MigrationMountMapping
                        key={`mounts:${projectReview.revision}`}
                        members={projectReview.members}
                        targetVolumes={targetVolumes}
                        target={projectReview.target}
                        source={projectReview.source.daemon}
                        onSaved={(path, contents, sha256) =>
                          setMappingFile({
                            revision: projectReview.revision,
                            path,
                            contents,
                            sha256,
                          })
                        }
                        mapping={mapping}
                        namespace={namespace}
                        onMappingChange={(mapping) =>
                          setMappingSelection({ revision, mapping })
                        }
                      />
                    </>
                  )}
                </>
              )}
              {volumes && (
                <>
                  <label>
                    Source volume
                    <select
                      disabled={busy}
                      value={volume}
                      onChange={(event) => {
                        setVolume(event.target.value);
                        setVolumeReview(null);
                        setError("");
                      }}
                    >
                      <option value="">Choose a volume</option>
                      {review.volumes.map((volume) => (
                        <option
                          key={volume.name}
                          value={volume.name}
                          disabled={
                            !volume.plainLocal || volume.name.length > 128
                          }
                        >
                          {volume.name}
                          {!volume.plainLocal
                            ? " · unsupported driver/options"
                            : volume.name.length > 128
                              ? " · unsupported name length"
                              : ""}
                        </option>
                      ))}
                    </select>
                  </label>
                  {review.volumes.length === 0 && (
                    <p>No source volumes to export.</p>
                  )}
                  <button
                    className="button"
                    disabled={busy || !volume || !connected}
                    onClick={checkVolume}
                  >
                    Check selected volume
                  </button>
                  {volumeReview && (
                    <p role="status">
                      Source metadata recorded. No running writable Docker
                      volume users were observed. Keep external writers stopped
                      until export completes.
                    </p>
                  )}
                </>
              )}
              {review.warnings.map((warning, index) => (
                <p key={index}>{warning}</p>
              ))}
            </>
          )}
          {projects && mappingDraft && !currentMappingFile && (
            <p role="status">
              Save the current mapping as a new Windows override before Windows
              validation, or save it through the connected WSL project for Linux
              validation. Changing destinations requires saving the new draft.
            </p>
          )}
          <div className="dialog-actions">
            <button
              className="button"
              disabled={
                !connected ||
                busy ||
                (projects
                  ? !projectReview || (!!mappingDraft && !currentMappingFile)
                  : volumes
                    ? !volumeReview
                    : !image || !reference)
              }
              onClick={continueExport}
            >
              {projects
                ? "Review Windows Compose files…"
                : "Choose export destination…"}
            </button>
            <button className="button" autoFocus onClick={close}>
              Close
            </button>
          </div>
        </dialog>
      )}
    </section>
  );
}
