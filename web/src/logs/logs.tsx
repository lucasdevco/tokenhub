import ServiceToolbar from "../service-toolbar";
import { SegmentedControl } from "../components/ui/segmented-control";
import { Button } from "../components/ui/button";
import { Badge } from "../components/ui/badge";
import { Input } from "../components/ui/input";
import { NativeSelect } from "../components/ui/native-select";
import { Alert, AlertDescription } from "../components/ui/alert";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "../components/ui/table";
import { Link, useLocation, useNavigate } from "@tanstack/react-router";
import Sidebar from "../sidebar";
import { listStateLabel, stateLabel } from "./state-label";
import { useEffect, useRef, useState } from "react";
import {
  bytes,
  downloadURL,
  get,
  timestamp,
  relativeTime,
  type CaptureFile,
  type FilePage,
  type RequestPage,
  type Transaction,
} from "./api";
import "./logs.css";
import CaptureDrawer from "./capture-drawer";
import RequestChart from "./request-chart";
import { callerLabel } from "./caller-label";

const PAGE = 100;
const columnHints: Record<string, string> = {
  "输入 Token": "输入 Token 总数（包含上游报告的缓存读取和写入）；未报告时留空",
  "输出 Token": "上游报告的输出 Token 数；未报告时留空",
  费用: "尚未配置模型价格",
  用量类型: "尚未定义用量分类",
  "生成速度 (tokens/s)":
    "输出 Token 数 ÷ 首个内容增量到生成结束的秒数；仅已完成的流式响应",
  "路由耗时 (ms)":
    "本地开始处理请求到发起上游 HTTP 请求的耗时，不含网络和上游等待",
  "首 Token 耗时 (ms)":
    "从请求开始到收到上游首个内容、思考或工具调用增量的耗时；压缩流无法可靠定位时留空",
  "总耗时 (ms)":
    "从 TokenHub 收到请求到响应结束的总耗时，包含流式输出全过程；未结束时留空，异常结束时显示实际耗时",
  结束原因: "上游的 finish_reason / stop_reason；Responses API 使用结束状态或 incomplete_details.reason",
};
const metric = (value: number | undefined, digits = 0) =>
  value === undefined || !Number.isFinite(value)
    ? "—"
    : value.toLocaleString(undefined, { maximumFractionDigits: digits });
function destination(item: Transaction) {
  const target =
    item.metadata.target_url ||
    item.metadata.source_url ||
    item.metadata_files?.[0] ||
    "未知请求";
  try {
    const url = new URL(target);
    return { full: target, path: url.pathname + url.search, host: url.host };
  } catch {
    return {
      full: target,
      path: target,
      host: item.metadata.id || "元数据不可用",
    };
  }
}

export default function Logs() {
  const [mode, setMode] = useState<"requests" | "files">("requests");
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("");
  const [hours, setHours] = useState("1");
  const [cursors, setCursors] = useState([""]);
  const offset = (cursors.length - 1) * PAGE;
  const cursor = cursors[cursors.length - 1];
  const [page, setPage] = useState<RequestPage | FilePage>();
  const [revision, setRevision] = useState(0);
  const [paused, setPaused] = useState(false);
  const pausedRef = useRef(false);
  const [online, setOnline] = useState(false);
  const [configurable, setConfigurable] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const key = useLocation({
    select: (location) =>
      typeof location.search.request === "string"
        ? location.search.request
        : undefined,
  });
  function setKey(request: string | undefined) {
    void navigate({
      to: ".",
      search: (previous) => {
        const next = { ...previous };
        if (request) next.request = request;
        else delete next.request;
        return next;
      },
      hash: (previous) => previous ?? "",
      replace: true,
    });
  }
  const [detail, setDetail] = useState<Transaction>();
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [file, setFile] = useState<CaptureFile>();
  const [selection, setSelection] = useState(0);
  function closeDrawer() {
    setKey(undefined);
    setFile(undefined);
    setDetail(undefined);
    setDetailError("");
  }
  useEffect(() => {
    document.title = "请求日志 · TokenHub";
    let active = true;
    const controller = new AbortController();
    get<{ settings: boolean }>("/api/capabilities", controller.signal)
      .then((value) => {
        if (active) setConfigurable(value.settings);
      })
      .catch(() => {});
    const events = new EventSource("/api/events");
    events.onopen = () => {
      if (active) {
        setOnline(true);
        if (!pausedRef.current) setRevision((value) => value + 1);
      }
    };
    events.onerror = () => {
      if (active) setOnline(false);
    };
    events.addEventListener("change", () => {
      if (active && !pausedRef.current) setRevision((value) => value + 1);
    });
    return () => {
      active = false;
      controller.abort();
      events.close();
    };
  }, []);
  useEffect(() => {
    pausedRef.current = paused;
    if (!paused) setRevision((value) => value + 1);
  }, [paused]);
  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(query);
      setCursors([""]);
    }, 200);
    return () => clearTimeout(timer);
  }, [query]);
  useEffect(() => {
    const controller = new AbortController();
    const params = new URLSearchParams({
      offset: "0",
      cursor,
      limit: String(PAGE),
      q: search,
    });
    if (mode === "requests") {
      params.set("state", filter);
      params.set("hours", hours);
    }
    setLoading(true);
    setError("");
    get<RequestPage | FilePage>(`/api/${mode}?${params}`, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setPage(result);
        setError(result.problem || "");
      })
      .catch((value) => {
        if (!controller.signal.aborted) setError(String(value));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [mode, cursor, search, filter, hours, revision]);
  useEffect(() => {
    if (!key) {
      setDetail(undefined);
      setDetailError("");
      setDetailLoading(false);
      return;
    }
    const controller = new AbortController();
    setDetailLoading(true);
    setDetailError("");
    get<Transaction>(
      `/api/requests/${encodeURIComponent(key)}`,
      controller.signal,
    )
      .then((result) => {
        if (!controller.signal.aborted) setDetail(result);
      })
      .catch((value) => {
        if (!controller.signal.aborted) setDetailError(String(value));
      })
      .finally(() => {
        if (!controller.signal.aborted) setDetailLoading(false);
      });
    return () => controller.abort();
  }, [key, revision, selection]);
  function selectRequest(item: Transaction) {
    setSelection((value) => value + 1);
    setDetailError("");
    setFile(undefined);
    setKey(item.key);
    setDetail(item);
  }
  function changeMode(next: "requests" | "files") {
    setMode(next);
    setCursors([""]);
    setPage(undefined);
    setKey(undefined);
    setDetail(undefined);
    setFile(undefined);
  }
  const requestPage =
    mode === "requests" ? (page as RequestPage | undefined) : undefined;
  const total = page?.total || 0;
  return (
    <div className="logs-app">
      <header className="topbar">
        <Link className="brand" to="/">
          TokenHub
        </Link>
        <span className={`status ${online ? "running" : "stopped"}`}>
          <i />
          {paused ? "列表已暂停" : online ? "实时更新" : "正在重连"}
        </span>
        <ServiceToolbar />
      </header>
      <div className="layout logs-layout">
        <Sidebar active="logs" settingsAvailable={configurable} />
        <main className="logs-main">
          <div className="logs-heading">
            <div>
              <h1>请求日志</h1>
            </div>
            {requestPage && (
              <div className="capture-counts">
                <span>{requestPage.all.toLocaleString()} 条记录</span>
                <span>{requestPage.unfinished.toLocaleString()} 未完成</span>
                <span>{requestPage.errors.toLocaleString()} 异常</span>
              </div>
            )}
          </div>
          {error && (
            <Alert variant="destructive" className="message error" role="alert">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {mode === "requests" && (
            <RequestChart
              histogram={requestPage?.histogram}
              hours={hours}
              onHoursChange={setHours}
              loading={loading}
              error={!!error}
            />
          )}
          <div className="logs-workspace">
            <section className="capture-library" aria-label="捕获记录">
              <div className="logs-toolbar">
                <SegmentedControl
                  aria-label="记录类型"
                  value={mode}
                  options={[
                    { value: "requests", label: "请求" },
                    { value: "files", label: "所有文件" },
                  ]}
                  onValueChange={(value) =>
                    changeMode(value as "requests" | "files")
                  }
                />
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setRevision((value) => value + 1)}
                >
                  刷新
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPaused((value) => !value)}
                >
                  {paused ? "恢复更新" : "暂停更新"}
                </Button>
              </div>
              <div className="capture-filters">
                <Input
                  type="search"
                  aria-label="搜索捕获记录"
                  placeholder={
                    mode === "requests"
                      ? "搜索 URL、ID、方法、状态…"
                      : "搜索文件名或归档…"
                  }
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
                <NativeSelect
                  aria-label="捕获状态"
                  value={filter}
                  disabled={mode === "files"}
                  onChange={(event) => {
                    setFilter(event.target.value);
                    setCursors([""]);
                  }}
                >
                  <option value="">全部状态</option>
                  <option value="unfinished">未完成</option>
                  <option value="completed">已完成</option>
                  <option value="error">请求异常</option>
                  <option value="capture_error">采集异常</option>
                </NativeSelect>
              </div>
              <div className="capture-table" aria-busy={loading}>
                <Table>
                  <TableHeader>
                    <TableRow>
                      {(mode === "requests"
                        ? [
                            "时间",
                            "请求",
                            "状态",
                            "模型",
                            "服务商",
                            "调用方",
                            "输入 Token",
                            "输出 Token",
                            "总耗时 (ms)",
                            "生成速度 (tokens/s)",
                            "路由耗时 (ms)",
                            "首 Token 耗时 (ms)",
                            "费用",
                            "用量类型",
                            "结束原因",
                          ]
                        : ["大小", "文件", "下载"]
                      ).map((title) => (
                        <TableHead key={title} title={columnHints[title]}>
                          {title}
                        </TableHead>
                      ))}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {mode === "requests"
                      ? (page as RequestPage)?.items.map((item) => {
                          const target = destination(item);
                          const failed =
                            item.state === "error" ||
                            item.capture_state === "failed" ||
                            item.capture_state === "incomplete" ||
                            (item.metadata.response_status_code || 0) >= 400;
                          return (
                            <TableRow
                              key={item.key}
                              className={key === item.key ? "current" : ""}
                              onClick={() => selectRequest(item)}
                            >
                              <TableCell className="capture-absolute-time">
                                <time
                                  dateTime={item.started}
                                  title={relativeTime(item.started)}
                                >
                                  {timestamp(item.started)}
                                </time>
                              </TableCell>
                              <TableCell>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="capture-link"
                                  title={`${item.metadata.method || "—"} ${target.full}`}
                                  aria-label={`${item.metadata.method || "—"} ${target.full}`}
                                  aria-haspopup="dialog"
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    selectRequest(item);
                                  }}
                                >
                                  <strong>
                                    <span className="method">
                                      {item.metadata.method || "—"}
                                    </span>{" "}
                                    {target.path}
                                  </strong>
                                </Button>
                              </TableCell>
                              <TableCell
                                className={`capture-state ${failed ? "failed" : item.state === "unfinished" ? "pending" : ""}`}
                                title={`${item.metadata.response_status_code || "—"} · ${stateLabel(item.state, item.capture_state)}`}
                              >
                                <Badge
                                  variant="secondary"
                                  className="http-status"
                                >
                                  {item.metadata.response_status_code || "—"}
                                </Badge>
                                <small>
                                  {" "}
                                  ·{" "}
                                  {listStateLabel(
                                    item.state,
                                    item.capture_state,
                                  )}
                                </small>
                              </TableCell>
                              <TableCell
                                className="capture-model"
                                title={item.metadata.model}
                              >
                                {item.metadata.model || "—"}
                              </TableCell>
                              <TableCell
                                className="capture-provider"
                                title={item.metadata.provider}
                              >
                                {item.metadata.provider || "—"}
                              </TableCell>
                              <TableCell
                                className="capture-caller"
                                title={item.metadata.user_agent}
                              >
                                {callerLabel(item.metadata.user_agent)}
                              </TableCell>
                              <TableCell className="capture-number">
                                {metric(item.metadata.input_tokens)}
                              </TableCell>
                              <TableCell className="capture-number">
                                {metric(item.metadata.output_tokens)}
                              </TableCell>
                              <TableCell className="capture-number">
                                {metric(
                                  item.state === "unfinished"
                                    ? undefined
                                    : item.duration_ms,
                                )}
                              </TableCell>
                              <TableCell className="capture-number">
                                {metric(item.metadata.tokens_per_second, 1)}
                              </TableCell>
                              <TableCell className="capture-number">
                                {metric(item.metadata.routing_overhead_ms, 2)}
                              </TableCell>
                              <TableCell className="capture-number">
                                {metric(
                                  item.metadata.time_to_first_token_ms,
                                  1,
                                )}
                              </TableCell>
                              <TableCell className="capture-number">
                                —
                              </TableCell>
                              <TableCell>—</TableCell>
                              <TableCell
                                className={`capture-finish ${failed ? "failed" : item.state === "unfinished" ? "pending" : ""}`}
                                title={`${item.metadata.response_status_code || "—"} · ${stateLabel(item.state, item.capture_state)}`}
                              >
                                {item.metadata.finish_reason || "—"}
                              </TableCell>
                            </TableRow>
                          );
                        })
                      : (page as FilePage)?.items.map((item) => (
                          <TableRow
                            key={item.name}
                            className={
                              file?.name === item.name ? "current" : ""
                            }
                          >
                            <TableCell>{bytes(item.size)}</TableCell>
                            <TableCell>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="capture-link"
                                onClick={() => {
                                  setKey(undefined);
                                  setDetail(undefined);
                                  setFile(item);
                                  setSelection((value) => value + 1);
                                  setDetailError("");
                                }}
                              >
                                <strong>{item.name}</strong>
                                <small>
                                  {item.problem || timestamp(item.modified)}
                                </small>
                              </Button>
                            </TableCell>
                            <TableCell>
                              <a
                                href={downloadURL(item.name)}
                                download={item.name}
                              >
                                下载
                              </a>
                            </TableCell>
                          </TableRow>
                        ))}
                  </TableBody>
                </Table>
                {!total && (
                  <div className="logs-empty">
                    {loading ? "正在加载记录…" : "没有匹配的捕获记录。"}
                  </div>
                )}
              </div>
              <div className="logs-pagination">
                <span>
                  {page?.items.length
                    ? `${offset + 1}–${offset + page.items.length} / ${total.toLocaleString()}`
                    : "0 条记录"}
                </span>
                <div>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={offset === 0 || loading}
                    onClick={() => setCursors((value) => value.slice(0, -1))}
                  >
                    上一页
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={!page?.next_cursor || loading}
                    onClick={() => {
                      if (page?.next_cursor)
                        setCursors((value) => [...value, page.next_cursor!]);
                    }}
                  >
                    下一页
                  </Button>
                </div>
              </div>
              {page?.indexing && (
                <p className="logs-footnote" role="status">
                  正在后台建立历史索引，当前展示已索引记录。
                </p>
              )}
              <p className="logs-footnote">
                只读查看。未完成记录可能来自实时请求或被中断的捕获。
              </p>
            </section>
          </div>
          <CaptureDrawer
            requestKey={key}
            detail={detail}
            file={file}
            detailLoading={detailLoading}
            detailError={detailError}
            revision={revision}
            selection={selection}
            onClose={closeDrawer}
          />
        </main>
      </div>
    </div>
  );
}
