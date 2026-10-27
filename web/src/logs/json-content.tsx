import { Button } from "../components/ui/button";
import { useMemo, useState } from "react";
import { jsonRows, type Expansion } from "./json-tree";

function JsonTree({ value }: { value: unknown }) {
  const [mode, setMode] = useState<Expansion>("default");
  const [overrides, setOverrides] = useState(new Map<string, boolean>());
  const [limit, setLimit] = useState(500);
  const [copyStatus, setCopyStatus] = useState("");
  const { rows, truncated } = useMemo(
    () => jsonRows(value, mode, overrides, limit),
    [value, mode, overrides, limit],
  );
  function expand(next: Expansion) {
    setMode(next);
    setOverrides(new Map());
    setLimit(500);
  }
  return (
    <div className="json-view">
      <div className="json-toolbar">
        <span>JSON</span>
        <div>
          <Button variant="outline" size="sm" onClick={() => expand("all")}>
            全部展开
          </Button>
          <Button variant="outline" size="sm" onClick={() => expand("none")}>
            全部收起
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(
                  JSON.stringify(value, null, 2),
                );
                setCopyStatus("已复制");
                setTimeout(() => setCopyStatus(""), 2000);
              } catch {
                setCopyStatus("复制失败，请重试");
              }
            }}
          >
            {copyStatus || "复制 JSON"}
          </Button>
        </div>
      </div>
      <div className="json-rows">
        {rows.map((row) => {
          const bracket = Array.isArray(row.value) ? ["[", "]"] : ["{", "}"];
          return (
            <div className="json-row" key={row.path}>
              <span className="json-indents" aria-hidden="true">
                {Array.from({ length: Math.min(row.depth, 24) }, (_, index) => (
                  <span key={index} />
                ))}
              </span>
              {row.container && !row.closing ? (
                <Button
                  variant="ghost"
                  size="sm"
                  className="json-toggle"
                  aria-expanded={row.open}
                  aria-label={
                    (row.open ? "收起" : "展开") + " " + (row.key || "JSON")
                  }
                  onClick={() => {
                    const next = new Map(overrides);
                    next.set(row.path, !row.open);
                    setOverrides(next);
                  }}
                >
                  <svg viewBox="0 0 16 16" aria-hidden="true">
                    <path d={row.open ? "m4 6 4 4 4-4" : "m6 4 4 4-4 4"} />
                  </svg>
                </Button>
              ) : (
                <span className="json-toggle-spacer" />
              )}
              {row.key !== null && (
                <span className="json-key">{JSON.stringify(row.key)}: </span>
              )}
              {row.container ? (
                <span className="json-summary">
                  {row.closing
                    ? bracket[1]
                    : row.open && row.count
                      ? bracket[0]
                      : `${bracket[0]} ${row.count} ${Array.isArray(row.value) ? "项" : "个字段"} ${bracket[1]}`}
                </span>
              ) : (
                <JsonValue value={row.value} />
              )}
            </div>
          );
        })}
      </div>
      {truncated && (
        <Button
          variant="outline"
          size="sm"
          className="json-more"
          onClick={() => setLimit((value) => value + 500)}
        >
          继续显示 500 行
        </Button>
      )}
    </div>
  );
}

function JsonValue({ value }: { value: unknown }) {
  const [expanded, setExpanded] = useState(false);
  const text = JSON.stringify(value) ?? "null";
  const long = text.length > 240;
  return (
    <span
      className={`json-value json-${value === null ? "null" : typeof value}`}
    >
      {long && !expanded ? text.slice(0, 240) + "…" : text}
      {long && (
        <Button
          variant="ghost"
          size="sm"
          className="json-string-toggle"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? "收起" : "展开完整内容"}
        </Button>
      )}
    </span>
  );
}

export default function JsonContent({ text }: { text: string }) {
  const parsed = useMemo(() => {
    try {
      return { valid: true, value: JSON.parse(text) };
    } catch {
      return { valid: false, value: null };
    }
  }, [text]);
  return parsed.valid ? <JsonTree value={parsed.value} /> : <pre>{text}</pre>;
}
