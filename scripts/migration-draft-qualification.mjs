// Qualification only: production input owner and prechecks, with explicitly
// supplied native fixture callbacks. Importing this module performs no I/O.
import assert from "node:assert/strict";
import {
  rememberComposeInputs,
  loadComposeInputs,
  composeInputKey,
} from "../src/model/compose-inputs.ts";
import { recheckMigrationDraft } from "../src/model/migration-draft.ts";
import { recheckWslDraft } from "../src/model/migration-wsl-draft.ts";

// Restart-style serialization evidence only. This deliberately does not claim a
// WebView profile write, actual app restart or interaction with the picker UI.
export function roundTripDraftInputs(inputs, bytes = null) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  let raw = bytes;
  try {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      value: {
        getItem: (key) => {
          assert.equal(key, "orbwin.compose.inputs.v1");
          return raw;
        },
        setItem: (key, value) => {
          assert.equal(key, "orbwin.compose.inputs.v1");
          raw = value;
        },
      },
    });
    rememberComposeInputs(inputs);
    const saved = raw;
    // Install a fresh storage object; the production owner has no cached jobs.
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      value: {
        getItem: () => saved,
        setItem: () => assert.fail("reopen must not write"),
      },
    });
    const reopened = loadComposeInputs().find(
      (value) => composeInputKey(value) === composeInputKey(inputs),
    );
    assert.deepEqual(reopened, inputs);
    return {
      bytes: saved,
      inputs: reopened,
      evidenceKind:
        "restart-style-input-serialization; not-WebView-or-app-restart",
    };
  } finally {
    if (previous) Object.defineProperty(globalThis, "localStorage", previous);
    else delete globalThis.localStorage;
  }
}

// Caller owns baselines, image/volume/container adoption guards, fixture byte
// writes, response-file acquisition and exact cleanup. No generic engine driver
// or new job truth is introduced here. All callbacks run sequentially.
export async function qualifyDraftReopen({
  inputs,
  readSource,
  inspectConnection,
  inspectMapping,
  validate,
  changeMapping,
  restoreMapping,
  removeOwnedSource,
  verifyBaselines,
  signal,
}) {
  const records = [];
  let validations = 0;
  const record = (step, value) => records.push({ step, ...value });
  const preflight = async (value) => {
    if (value.kind === "wsl")
      return recheckWslDraft(
        value,
        "validate",
        inspectConnection,
        readSource,
        signal,
      );
    await recheckMigrationDraft(
      value.migration,
      value,
      "validate",
      readSource,
      inspectMapping,
      signal,
    );
  };
  const dispatch = async (value) => {
    const mappingReview = await preflight(value);
    if (signal.aborted)
      throw Error("Qualification cancelled before native Validate");
    validations++;
    const result = await validate(value, mappingReview);
    assert.equal(result.success, true);
    assert.equal(result.mutation, false);
    return result;
  };
  await verifyBaselines("before");
  assert.ok(
    inputs.migration?.mapping,
    "This bounded journey requires its own saved fixture mapping",
  );
  assert.equal(inputs.project.startsWith("orbwin-draft-"), true);
  await dispatch(inputs);
  record("initial-production-Validate", { success: true });
  const reopened = roundTripDraftInputs(inputs);
  record("restart-style-reload", { evidenceKind: reopened.evidenceKind });
  await dispatch(reopened.inputs);
  record("reopened-production-Validate", { success: true });
  const accepted = validations;
  let mappingChanged = false;
  try {
    mappingChanged = true;
    await changeMapping();
    if (inputs.kind === "wsl") {
      // WSL byte refusal occurs inside the actual guest/native Validate before any
      // Up/engine mutation; Windows byte refusal precedes Compose dispatch entirely.
      const mappingReview = await preflight(reopened.inputs);
      const refused = await validate(reopened.inputs, mappingReview);
      assert.equal(refused.success, false);
      assert.equal(refused.mutation, false);
      assert.match(
        refused.stdout ?? refused.error ?? "",
        /saved mapping bytes changed/i,
      );
      record("changed-mapping", {
        refused: true,
        boundary: "guest-before-configuration-resolution",
      });
    } else {
      await assert.rejects(() => dispatch(reopened.inputs));
      assert.equal(validations, accepted);
      record("changed-mapping", {
        refused: true,
        boundary: "native-inspection-before-Compose-dispatch",
      });
    }
  } finally {
    if (mappingChanged) await restoreMapping();
  }
  await dispatch(reopened.inputs);
  record("restored-fixture-mapping-Validate", { success: true });
  // Source-container removal is the LAST destructive fixture step: the original
  // immutable container ID cannot be recreated, and old hints must stay refused.
  const beforeSource = validations;
  await removeOwnedSource();
  await assert.rejects(() => dispatch(reopened.inputs));
  assert.equal(validations, beforeSource);
  record("changed-source", {
    refused: true,
    boundary: "frontend-preflight-before-native-Validate",
  });
  const cancelled = new AbortController();
  cancelled.abort();
  if (inputs.kind === "wsl")
    await assert.rejects(() =>
      recheckWslDraft(
        reopened.inputs,
        "validate",
        () => assert.fail("cancelled connection read"),
        () => assert.fail("cancelled source read"),
        cancelled.signal,
      ),
    );
  else
    await assert.rejects(() =>
      recheckMigrationDraft(
        reopened.inputs.migration,
        reopened.inputs,
        "validate",
        () => assert.fail("cancelled source read"),
        () => assert.fail("cancelled file read"),
        cancelled.signal,
      ),
    );
  record("cancel-before-read", { refused: true });
  await verifyBaselines("fixtures-only-before-cleanup");
  return {
    schema: 1,
    records,
    validations,
    uiRestartQualified: false,
    upQualified: false,
    cleanupOwnedByCaller: true,
  };
}
