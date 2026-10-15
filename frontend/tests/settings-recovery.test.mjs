import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import ts from "typescript";

const source = await readFile(new URL("../src/settings-recovery-policy.ts", import.meta.url), "utf8");
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const { settingsRecoveryPolicy: policy } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`);
const pending = { state: "pending", code: "credential_activation_pending", can_reload: true };
const active = { state: "active", code: "", can_reload: true };
const cfg = (activation) => activation === undefined ? { config_revision: 42 } : { config_revision: 42, repository_activation: activation };

test("unbound settings can save drafts and expose no recovery", () => {
  assert.equal(policy(cfg(), true, false, false).canSave, true);
  assert.equal(policy(cfg(), true, false, false).showRecovery, false);
  assert.equal(policy(cfg(), false, false, false).canReload, false);
});
test("active credentials keep normal editing without recovery", () => {
  assert.equal(policy(cfg(active), true, false, false).canSave, true);
  assert.equal(policy(cfg(active), true, false, false).showRecovery, false);
});
test("pending credentials permit only a clean explicit reload", () => {
  const p = policy(cfg(pending), false, false, false);
  assert.equal(p.canReload, true);
  assert.equal(p.canSave, false);
  assert.equal(p.canEdit, false);
  assert.equal(policy(cfg(pending), true, false, false).canReload, false);
});
test("busy and result-unknown locks prevent all writes and edits", () => {
  for (const a of [undefined, active, pending]) for (const [locked, busy] of [[true,false],[false,true],[true,true]]) {
    const p = policy(cfg(a), true, locked, busy);
    assert.equal(p.canSave || p.canEdit || p.canReload, false);
    assert.equal(policy(cfg(a), false, locked, busy).canReload, false);
  }
});
test("absent snapshots grant no action", () => {
  const p = policy(null, false, false, false);
  assert.equal(p.canEdit || p.canSave || p.canReload || p.showRecovery, false);
});
test("unsafe and noncanonical numeric revisions cannot authorize either path", () => {
  for (const revision of [0, -1, 1.5, 2**53, Number.MAX_VALUE, Infinity, NaN, "42", null, undefined]) {
    for (const a of [undefined, active, pending]) {
      const p = policy({ ...cfg(a), config_revision: revision }, false, false, false);
      assert.equal(p.reason, "unsafe_revision");
      assert.equal(p.canEdit || p.canSave || p.canReload, false);
    }
  }
  assert.equal(policy({ ...cfg(pending), config_revision: Number.MAX_SAFE_INTEGER }, false, false, false).canReload, true);
});
test("null and malformed status must not fall back to legacy save", () => {
  for (const a of [null, {}, [], "active", { ...active, can_reload: 1 }, { ...pending, can_reload: "true" }, { ...active, code: "settings_changed" }, { ...pending, code: "" }, { state: "future", code: "", can_reload: true }]) {
    const p = policy(cfg(a), true, false, false);
    assert.equal(p.reason, "invalid_activation");
    assert.equal(p.canEdit || p.canSave || p.canReload, false);
  }
  assert.equal(policy({ ...cfg(), repository_activation: undefined }, true, false, false).reason, "invalid_activation");
});
test("unavailable is read-only and contradictory grants are rejected", () => {
  for (const code of ["credential_activation_unavailable", "settings_changed"]) {
    const p = policy(cfg({ state: "unavailable", code, can_reload: false }), false, false, false);
    assert.equal(p.showRecovery, true);
    assert.equal(p.canEdit || p.canSave || p.canReload, false);
    assert.equal(policy(cfg({ state: "unavailable", code, can_reload: true }), false, false, false).reason, "invalid_activation");
  }
});
test("server withholding reload cannot be overridden by a pending label", () => {
  assert.equal(policy(cfg({ ...pending, can_reload: false }), false, false, false).canReload, false);
});
test("clean ordinary settings cannot be submitted repeatedly", () => {
  assert.equal(policy(cfg(active), false, false, false).canSave, false);
});
