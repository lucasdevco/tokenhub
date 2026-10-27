import { SegmentedControl } from "../components/ui/segmented-control";
import { Button } from "../components/ui/button";
import { Alert, AlertDescription } from "../components/ui/alert";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { bytes, downloadURL } from "./api";
import { useCapture, type View } from "./use-capture";
import JsonContent from "./json-content";
import { pretty } from "./decoders";
import { ChevronDown, ChevronUp } from "lucide-react";

export default function CapturePanel({
  title,
  name,
  revision,
  selection,
  file = false,
  collapseBody = false,
}: {
  title: string;
  name?: string;
  revision: number;
  selection: number;
  file?: boolean;
  collapseBody?: boolean;
}) {
  const [view, setView] = useState<View>("raw");
  const [following, setFollowing] = useState(true);
  const [rawOffset, setRawOffset] = useState(0);
  const streamArea = useRef<HTMLDivElement>(null);
  const followScroll = useRef(true);
  const body = useRef<HTMLDivElement>(null);
  const bodyId = useId();
  const [bodyExpanded, setBodyExpanded] = useState(false);
  const [bodyLong, setBodyLong] = useState(false);
  const capture = useCapture(
    name,
    view,
    following,
    rawOffset,
    revision,
    selection,
  );
  const response = title === "响应" && !file;
  const collapsible = collapseBody && view !== "hex";
  const rawRequest = useMemo(() => {
    const text = capture.raw?.text || "";
    const separator = /\r?\n\r?\n/.exec(text);
    if (
      view !== "raw" ||
      !separator ||
      (!(collapseBody && /^[A-Z]+ \S+ HTTP\/\d(?:\.\d)?\r?\n/.test(text)) &&
        !(response && /^HTTP\/\d(?:\.\d)? \d{3}\b/.test(text)))
    )
      return { headers: "", body: text, emptyBody: text.length === 0 };
    const body = text.slice(separator.index + separator[0].length);
    return {
      headers: collapseBody ? text.slice(0, separator.index) : "",
      body: collapseBody
        ? pretty(body)
        : text.slice(0, separator.index + separator[0].length) + pretty(body),
      emptyBody: body.length === 0,
    };
  }, [capture.raw?.text, collapseBody, response, view]);
  const responseEmpty =
    response &&
    !capture.error &&
    (!name ||
      (view === "stream"
        ? !capture.preview.text && capture.preview.messages.length === 0
        : capture.raw &&
          (view === "raw"
            ? rawRequest.emptyBody
            : capture.raw.page.size === 0)));
  useEffect(() => {
    const area = streamArea.current;
    if (area && !collapseBody && followScroll.current)
      area.scrollTop = area.scrollHeight;
  }, [capture.preview, collapseBody]);
  useEffect(() => {
    setBodyExpanded(false);
  }, [name, selection, view]);
  useEffect(() => {
    const content = body.current;
    if (!collapsible || !content) return;
    const measure = () => {
      const lineHeight = parseFloat(getComputedStyle(content).lineHeight);
      setBodyLong(content.getBoundingClientRect().height > lineHeight * 20 + 1);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(content);
    return () => observer.disconnect();
  }, [collapsible]);
  return (
    <section className="capture-panel" aria-label={title}>
      <div className="logs-toolbar">
        <h3>{title}</h3>
        {name && (
          <a href={downloadURL(name)} download={name}>
            下载完整文件
          </a>
        )}
      </div>
      <div className="capture-controls">
        <SegmentedControl
          className="capture-view-options"
          aria-label="查看方式"
          value={view}
          onValueChange={(value) => {
            setView(value as View);
            setRawOffset(0);
          }}
          options={[
            { value: "stream", label: "实时预览 / JSON" },
            { value: "raw", label: "原始文本" },
            { value: "hex", label: "十六进制" },
          ].filter((option) => !file || option.value !== "stream")}
        />
        {view === "stream" && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setFollowing((value) => !value)}
          >
            {following ? "暂停流" : "继续流"}
          </Button>
        )}
        <span role="status">{capture.status}</span>
      </div>
      {capture.error && (
        <Alert variant="destructive" className="message error" role="alert">
          <AlertDescription>{capture.error}</AlertDescription>
        </Alert>
      )}
      {view === "hex" && name && (
        <div className="logs-pagination">
          <Button
            variant="outline"
            size="sm"
            disabled={rawOffset === 0}
            onClick={() => setRawOffset((value) => Math.max(0, value - 65536))}
          >
            上 64 KiB
          </Button>
          <span>
            {capture.raw
              ? `${capture.raw.page.offset.toLocaleString()}–${capture.raw.page.next.toLocaleString()} / ${bytes(capture.raw.page.size)}`
              : "正在读取…"}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={
              !capture.raw || capture.raw.page.next >= capture.raw.page.size
            }
            onClick={() => setRawOffset((value) => value + 65536)}
          >
            下 64 KiB
          </Button>
        </div>
      )}
      {view === "stream" && (
        <p className="logs-footnote">
          预览保留最近 2 MiB 文本和最多 300 个事件；原始文件不受限制。
        </p>
      )}
      <div
        className="capture-content"
        ref={streamArea}
        tabIndex={0}
        onScroll={(event) => {
          const area = event.currentTarget;
          followScroll.current =
            area.scrollHeight - area.scrollTop - area.clientHeight < 100;
        }}
      >
        {view === "stream" && capture.preview.headers && (
          <details className="capture-headers">
            <summary>HTTP 请求头 / 响应头</summary>
            <pre>{capture.preview.headers}</pre>
          </details>
        )}
        {rawRequest.headers && (
          <pre className="capture-request-headers">{rawRequest.headers}</pre>
        )}
        <div
          id={bodyId}
          className={
            collapsible && !bodyExpanded ? "capture-content-collapsed" : ""
          }
        >
          <div className="capture-body" ref={body}>
            {view === "stream" ? (
              <>
                {capture.preview.text && (
                  <JsonContent
                    key={`${name}:${selection}`}
                    text={capture.preview.text}
                  />
                )}
              </>
            ) : (
              <pre>{rawRequest.body}</pre>
            )}
            {view === "stream" &&
              capture.preview.messages.map((message) => (
                <article className="stream-message" key={message.id}>
                  <div>{message.label}</div>
                  <JsonContent
                    key={`${name}:${selection}:${message.id}`}
                    text={message.text}
                  />
                </article>
              ))}
          </div>
        </div>
      </div>
      {responseEmpty && (
        <p className="capture-body-empty" role="status">
          暂无响应体
        </p>
      )}
      {response &&
        name &&
        view !== "stream" &&
        !capture.raw &&
        !capture.error && (
          <p className="capture-body-empty" role="status">
            正在加载响应体…
          </p>
        )}
      {collapsible && bodyLong && (
        <Button
          variant="link"
          size="sm"
          className="capture-body-toggle"
          aria-expanded={bodyExpanded}
          aria-controls={bodyId}
          onClick={() => setBodyExpanded((value) => !value)}
        >
          {bodyExpanded ? (
            <ChevronUp aria-hidden="true" />
          ) : (
            <ChevronDown aria-hidden="true" />
          )}
          {bodyExpanded
            ? `收起${title} body`
            : `展开完整${title} body（默认展示前 20 行）`}
        </Button>
      )}
    </section>
  );
}
