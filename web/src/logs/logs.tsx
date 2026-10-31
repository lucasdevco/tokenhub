import { getLocale, t, useLocale } from "../i18n";
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

const metric = (value: number | undefined, digits = 0) =>
  value === undefined || !Number.isFinite(value)
    ? "—"
    : value.toLocaleString(getLocale(), { maximumFractionDigits: digits });
function destination(item: Transaction) {
  const target =
    item.metadata.target_url ||
    item.metadata.source_url ||
    item.metadata_files?.[0] ||
    t("Unknown request");
  try {
    const url = new URL(target);
    return { full: target, path: url.pathname + url.search, host: url.host };
  } catch {
    return {
      full: target,
      path: target,
      host: item.metadata.id || t("Metadata unavailable"),
    };
  }
}

export default function Logs() {
  const locale = useLocale();
  useEffect(() => {
    document.title = t("Request logs · TokenHub");
  }, [locale]);
  const columnHints: Record<string, string> = {
    [t("Input tokens")]: t(
      "Total input tokens, including cache reads and writes reported by the upstream. Blank when unreported.",
    ),
    [t("Output tokens")]: t(
      "Output tokens reported by the upstream. Blank when unreported.",
    ),
    [t("Cost")]: t("Model pricing is not configured"),
    [t("Usage type")]: t("Usage categories are not defined"),
    [t("Generation speed (tokens/s)")]: t(
      "Output tokens divided by seconds from the first content delta to generation end. Completed streaming responses only.",
    ),
    [t("Routing time (ms)")]: t(
      "Time from local request handling to the upstream HTTP request, excluding network and upstream wait time.",
    ),
    [t("Time to first token (ms)")]: t(
      "Time from request start to the first upstream content, reasoning, or tool call delta. Blank when compressed streams prevent reliable measurement.",
    ),
    [t("Total time (s)")]: t(
      "Time from request receipt to response completion, including streaming. Blank while pending. Shows actual duration for interrupted requests.",
    ),
    [t("Finish reason")]: t(
      "Upstream finish_reason / stop_reason. Responses API uses the final status or incomplete_details.reason.",
    ),
  };
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
          {paused
            ? t("List paused")
            : online
              ? t("Live updates")
              : t("Reconnecting")}
        </span>
        <ServiceToolbar />
      </header>
      <div className="layout logs-layout">
        <Sidebar active="logs" settingsAvailable={configurable} />
        <main className="logs-main">
          <div className="logs-heading">
            <div>
              <h1>{t("Request logs")}</h1>
            </div>
            {requestPage && (
              <div className="capture-counts">
                <span>
                  {requestPage.all.toLocaleString(getLocale())} {t("records")}
                </span>
                <span>
                  {requestPage.unfinished.toLocaleString(getLocale())}{" "}
                  {t("unfinished")}
                </span>
                <span>
                  {requestPage.errors.toLocaleString(getLocale())} {t("errors")}
                </span>
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
            <section
              className="capture-library"
              aria-label={t("Capture records")}
            >
              <div className="logs-toolbar">
                <SegmentedControl
                  aria-label={t("Record type")}
                  value={mode}
                  options={[
                    { value: "requests", label: t("Request") },
                    { value: "files", label: t("All files") },
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
                  {t("Refresh")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPaused((value) => !value)}
                >
                  {paused ? t("Resume updates") : t("Pause updates")}
                </Button>
              </div>
              <div className="capture-filters">
                <Input
                  type="search"
                  aria-label={t("Search captures")}
                  placeholder={
                    mode === "requests"
                      ? t("Search URL, ID, method, status…")
                      : t("Search file name or archive…")
                  }
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
                <NativeSelect
                  aria-label={t("Capture status")}
                  value={filter}
                  disabled={mode === "files"}
                  onChange={(event) => {
                    setFilter(event.target.value);
                    setCursors([""]);
                  }}
                >
                  <option value="">{t("All statuses")}</option>
                  <option value="unfinished">{t("unfinished")}</option>
                  <option value="completed">{t("Completed")}</option>
                  <option value="error">{t("Request error")}</option>
                  <option value="capture_error">{t("Capture error")}</option>
                </NativeSelect>
              </div>
              <div className="capture-table" aria-busy={loading}>
                <Table>
                  <TableHeader>
                    <TableRow>
                      {(mode === "requests"
                        ? [
                            t("Time"),
                            t("Request"),
                            t("Status"),
                            t("Model"),
                            t("Provider"),
                            t("Caller"),
                            t("Input tokens"),
                            t("Output tokens"),
                            t("Total time (s)"),
                            t("Generation speed (tokens/s)"),
                            t("Routing time (ms)"),
                            t("Time to first token (ms)"),
                            t("Cost"),
                            t("Usage type"),
                            t("Finish reason"),
                          ]
                        : [t("Size"), t("File"), t("Download")]
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
                                    : item.duration_ms / 1000,
                                  2,
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
                                {t("Download")}
                              </a>
                            </TableCell>
                          </TableRow>
                        ))}
                  </TableBody>
                </Table>
                {!total && (
                  <div className="logs-empty">
                    {loading
                      ? t("Loading records…")
                      : t("No matching capture records.")}
                  </div>
                )}
              </div>
              <div className="logs-pagination">
                <span>
                  {page?.items.length
                    ? `${offset + 1}–${offset + page.items.length} / ${total.toLocaleString(getLocale())}`
                    : t("0 records")}
                </span>
                <div>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={offset === 0 || loading}
                    onClick={() => setCursors((value) => value.slice(0, -1))}
                  >
                    {t("Previous")}
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
                    {t("Next")}
                  </Button>
                </div>
              </div>
              {page?.indexing && (
                <p className="logs-footnote" role="status">
                  {t(
                    "Building the history index in the background. Showing indexed records.",
                  )}
                </p>
              )}
              <p className="logs-footnote">
                {t(
                  "Read-only view. Unfinished records may be active requests or interrupted captures.",
                )}
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
