import { assert, test } from "vitest";
import { readDraft, updateCredential } from "./config.ts";

test("unresolved aliases and non-map drafts remain recoverable editor errors", () => {
  const invalid = readDraft("server: *missing");
  assert.strictEqual(invalid.config, null);
  assert.match(invalid.error, /alias/i);
  assert.match(readDraft("scalar").error, /映射/);
  const valid = readDraft("server:\n  port: 5601");
  assert.strictEqual(valid.error, "");
  assert.strictEqual(valid.config?.server?.port, 5601);
});
test("clearing final credential removes authorization while keeping unrelated fields and comments", () => {
  const original =
    "# retain this comment\ncustom: preserved\nproviders:\n  local:\n    authorization:\n      header: Authorization\n      backend_key: ${BACKEND_KEY}\n      client_key: ${CLIENT_KEY}\n";
  const first = updateCredential(original, "local", "backend_key", "");
  assert.strictEqual(
    readDraft(first).config?.providers?.local.authorization?.client_key,
    "${CLIENT_KEY}",
  );
  const final = updateCredential(first, "local", "client_key", "");
  assert.strictEqual(
    readDraft(final).config?.providers?.local.authorization,
    undefined,
  );
  assert.strictEqual(readDraft(final).config?.custom, "preserved");
  assert.match(final, /# retain this comment/);
});
