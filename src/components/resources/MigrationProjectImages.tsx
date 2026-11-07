import { useState } from "react";
import type { migrationProjectReview } from "../../model/migration-projects";

export function MigrationProjectImages({
  review,
  busy,
  onExport,
}: {
  review: ReturnType<typeof migrationProjectReview>;
  busy: boolean;
  onExport: (id: string, reference: string) => void;
}) {
  const [references, setReferences] = useState<Record<string, string>>({});
  return (
    <details>
      <summary>Images observed in ordinary project members</summary>
      <p>
        These are the actual images used by observed members. They do not
        establish the current contents of Compose files or mutable tags. Export
        is separate from reviewed import, volume transfer and project startup;
        source images and workloads are retained. Return to this project review
        after export. Its observed snapshot and Windows mapping choices remain
        until Refresh or Close; refresh before relying on changed source state.
      </p>
      {review.unknownImages > 0 && (
        <p role="status">
          {review.unknownImages} ordinary member(s) have no image identity.
          Refresh the source; no image was guessed.
        </p>
      )}
      {!review.images.length && (
        <p>
          No immutable image identities were observed for ordinary services.
        </p>
      )}
      {review.images.map((image) => {
        const reference = references[image.id] ?? image.tags[0] ?? image.id;
        return (
          <section
            key={image.id}
            aria-label={`Observed source image ${image.id}`}
          >
            <p>
              {image.services.join(", ") || "Service label missing"}:{" "}
              <code>{image.id}</code>
            </p>
            {image.available ? (
              <>
                <label>
                  Source image reference for export
                  <select
                    disabled={busy}
                    value={reference}
                    onChange={(event) =>
                      setReferences({
                        ...references,
                        [image.id]: event.target.value,
                      })
                    }
                  >
                    {image.tags.map((tag) => (
                      <option key={tag} value={tag}>
                        {tag}
                      </option>
                    ))}
                    <option value={image.id}>
                      Immutable image ID (no tag handoff)
                    </option>
                  </select>
                </label>
                <button
                  className="button"
                  disabled={busy}
                  onClick={() => onExport(image.id, reference)}
                >
                  Review image export…
                </button>
              </>
            ) : (
              <p role="status">
                This member image was absent from the same source inventory.
                Refresh and review it again before export.
              </p>
            )}
          </section>
        );
      })}
    </details>
  );
}
