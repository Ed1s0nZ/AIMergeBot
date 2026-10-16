import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import ts from "typescript";
const source = await readFile(
  new URL("../src/repository-availability.ts", import.meta.url),
  "utf8",
);
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const { repositoryManagementAvailable: available } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`
);
test("unready, old and malformed responses never mount repository creation", () => {
  for (const value of [
    undefined,
    null,
    false,
    [],
    {},
    { available: false, reason: "native_audit_not_ready" },
    { available: true },
    { available: "true", reason: "" },
    { available: 1, reason: "" },
    { available: true, reason: null },
    { available: true, reason: "unknown" },
    { available: true, reason: "native_audit_not_ready" },
  ])
    assert.equal(available(value), false);
});
test("ready requires an explicit boolean and empty reason", () => {
  assert.equal(available({ available: true, reason: "" }), true);
});
