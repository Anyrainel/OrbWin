import type { ComposeInputs } from "./compose-inputs";
import type { MigrationImageReview } from "./migration-images";
import { composeInputs } from "../components/resources/model.ts";

// Source labels are hints. This builds an editable validation form only, never
// opens a path, creates a workload, infers profiles or copies env/credentials.
export function migrationProjectReview(
  inventory: MigrationImageReview,
  project: string,
) {
  const members = inventory.projectMembers.filter(
    (member) => member.project === project,
  );
  if (!members.length)
    throw new Error("Select a project from the current source review.");
  const coherent = composeInputs(
    project,
    members.map((member) => ({
      Labels: {
        "com.docker.compose.project": member.project,
        "com.docker.compose.project.config_files": member.configFilesLabel,
        "com.docker.compose.project.working_dir": member.workingDirectory,
        "com.docker.compose.project.environment_file": member.explicitEnvFiles
          ? "explicit selection requires review"
          : "",
      },
    })),
  );
  const inputs: ComposeInputs = coherent ?? {
    project,
    file: "",
    overrides: [],
    profiles: [],
    directory: "",
  };
  const ordinary = members.filter((member) => !member.oneOff);
  const imageIDs = [
    ...new Set(
      ordinary
        .map((member) => member.image)
        .filter((image): image is string => !!image),
    ),
  ].sort();
  return {
    source: structuredClone(inventory.source),
    target: inventory.target,
    inputs,
    suggestedFiles: !!coherent,
    members: structuredClone(members),
    services: [
      ...new Set(
        members
          .filter((member) => !member.oneOff && member.service)
          .map((member) => member.service),
      ),
    ].sort(),
    images: imageIDs.map((id) => {
      const observed = inventory.images.find((image) => image.id === id);
      return {
        id,
        services: [
          ...new Set(
            ordinary
              .filter((member) => member.image === id)
              .map((member) => member.service)
              .filter(Boolean),
          ),
        ].sort(),
        tags: [...(observed?.tags ?? [])].filter(
          (tag) => tag && tag !== "<none>:<none>",
        ),
        available: !!observed,
      };
    }),
    unknownImages: ordinary.filter((member) => !member.image).length,
    volumes: [...new Set(members.flatMap((member) => member.volumes))].sort(),
    bindMounts: members.some((member) => member.bindMounts),
    explicitEnvFiles: members.some((member) => member.explicitEnvFiles),
    message: coherent
      ? "Ordered Windows paths agree across the observed source members. Review them and select only trusted files; labels do not prove files still exist or have unchanged content."
      : "Source paths are missing, conflicting, Linux/UNC paths, or use explicit env files. Enter reviewed Windows Compose and interpolation env-file paths manually, or connect the original project through WSL distributions. No file set was guessed.",
  };
}
