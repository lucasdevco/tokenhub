export type Expansion = "default" | "all" | "none";
export type JsonRow = {
  path: string;
  key: string | null;
  value: unknown;
  depth: number;
  container: boolean;
  open: boolean;
  closing: boolean;
  count: number;
};

type ValueTask = {
  kind: "value";
  path: string;
  key: string | null;
  value: unknown;
  depth: number;
};
type ChildrenTask = {
  kind: "children";
  path: string;
  value: Record<string, unknown> | unknown[];
  depth: number;
  keys: string[] | null;
  index: number;
  count: number;
};

// Project only visible rows. Traversal is iterative and bounded even for deep JSON.
export function jsonRows(
  value: unknown,
  mode: Expansion,
  overrides: ReadonlyMap<string, boolean>,
  limit = 500,
) {
  const rows: JsonRow[] = [];
  const tasks: (ValueTask | ChildrenTask)[] = [
    { kind: "value", path: "$", key: null, value, depth: 0 },
  ];
  while (tasks.length && rows.length < limit) {
    const task = tasks.pop()!;
    if (task.kind === "children") {
      if (task.index === task.count) {
        rows.push({
          path: task.path + ":end",
          key: null,
          value: task.value,
          depth: task.depth,
          container: true,
          open: true,
          closing: true,
          count: task.count,
        });
        continue;
      }
      const key = task.keys ? task.keys[task.index] : String(task.index);
      tasks.push({ ...task, index: task.index + 1 });
      tasks.push({
        kind: "value",
        path: task.path + "[" + JSON.stringify(key) + "]",
        key,
        value: (task.value as Record<string, unknown>)[key],
        depth: task.depth + 1,
      });
      continue;
    }
    const container = typeof task.value === "object" && task.value !== null;
    const keys =
      container && !Array.isArray(task.value)
        ? Object.keys(task.value as object)
        : null;
    const count = container
      ? Array.isArray(task.value)
        ? task.value.length
        : keys!.length
      : 0;
    const open =
      container &&
      (overrides.get(task.path) ??
        (mode === "all" || (mode === "default" && task.depth === 0)));
    rows.push({ ...task, container, open, closing: false, count });
    if (open && count)
      tasks.push({
        kind: "children",
        path: task.path,
        value: task.value as Record<string, unknown>,
        depth: task.depth,
        keys,
        index: 0,
        count,
      });
  }
  return { rows, truncated: tasks.length > 0 };
}
