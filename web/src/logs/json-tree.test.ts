import { assert, test } from "vitest";
import { jsonRows } from "./json-tree.ts";

test("JSON supports per-path overrides, full collapse and large-array pagination", () => {
  const value = {
    messages: [{ role: "user", content: "hello" }],
    model: "example",
  };
  const initial = jsonRows(value, "default", new Map());
  assert.strictEqual(
    initial.rows.find((row) => row.key === "messages")?.open,
    false,
  );
  const expanded = jsonRows(value, "all", new Map());
  assert.ok(expanded.rows.some((row) => row.key === "content"));
  const collapsed = jsonRows(value, "none", new Map());
  assert.strictEqual(collapsed.rows.length, 1);
  const override = jsonRows(value, "all", new Map([['$["messages"]', false]]));
  assert.ok(!override.rows.some((row) => row.key === "content"));
  const large = jsonRows(
    Array.from({ length: 10000 }, (_, index) => index),
    "all",
    new Map(),
  );
  assert.strictEqual(large.rows.length, 500);
  assert.strictEqual(large.truncated, true);
});
test("deep JSON traversal is bounded and does not recurse through the JS stack", () => {
  let value: unknown = 0;
  for (let depth = 0; depth < 10000; depth++) value = { child: value };
  const result = jsonRows(value, "all", new Map(), 500);
  assert.strictEqual(result.rows.length, 500);
  assert.strictEqual(result.truncated, true);
});
