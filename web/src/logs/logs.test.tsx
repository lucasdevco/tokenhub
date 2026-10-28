// @vitest-environment jsdom
import {
  cleanup,
  within,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RouterProvider } from "@tanstack/react-router";
import { createAppRouter } from "../router";
import type { Transaction } from "./api";

const transaction: Transaction = {
  key: "request-1",
  metadata: {
    method: "GET",
    target_url: "https://example.com/api",
    response_status_code: 200,
  },
  started: "2026-10-12T10:00:00Z",
  state: "completed",
  request_bytes: 20,
  response_bytes: 30,
  duration_ms: 10,
};
const eventSources: { close: ReturnType<typeof vi.fn> }[] = [];
const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
  const path = String(input);
  if (path === "/api/capabilities") return Response.json({ settings: true });
  if (path === "/api/session") return Response.json({ token: "session" });
  if (path === "/api/config")
    return Response.json({
      yaml: "providers:\n  local:\n    base_url: https://example.com\n    active_account: personal\n    accounts: [{id: personal, name: 个人, key: test-key}]\n",
      version: "v1",
    });
  if (path === "/api/status")
    return Response.json({
      state: "running",
      pending: false,
      active_provider: "local",
    });
  if (path.startsWith("/api/requests?"))
    return Response.json({
      items: [transaction],
      total: 1,
      all: 1,
      unfinished: 0,
      errors: 0,
      histogram: {
        buckets: [{ started: transaction.started, count: 120 }],
        interval_seconds: 1800,
        total: 120,
      },
    });
  if (path === "/api/requests/request-1") return Response.json(transaction);
  if (path === "/api/requests/outside%2Fpage")
    return Response.json({ ...transaction, key: "outside/page" });
  throw Error(`Unexpected request: ${path}`);
});

beforeEach(() => {
  history.replaceState(null, "", "/logs/");
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  fetchMock.mockClear();
  eventSources.length = 0;
  vi.stubGlobal("fetch", fetchMock);
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  vi.stubGlobal(
    "EventSource",
    class {
      close = vi.fn();
      constructor() {
        eventSources.push(this);
      }
      addEventListener() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test("selecting a request opens its capture panels and cancelling closes the drawer", async () => {
  const { unmount } = render(<RouterProvider router={createAppRouter()} />);
  fireEvent.click(await screen.findByRole("button", { name: /example.com/ }));
  await waitFor(() =>
    expect(new URLSearchParams(location.search).get("request")).toBe(
      transaction.key,
    ),
  );
  const dialog = await screen.findByRole("dialog", { name: "请求详情" });
  expect(screen.getByRole("heading", { name: "GET · 200" })).toBeTruthy();
  expect(screen.getByRole("region", { name: "请求" })).toBeTruthy();
  expect(screen.getByRole("region", { name: "响应" })).toBeTruthy();
  fireEvent.keyDown(dialog, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(new URLSearchParams(location.search).has("request")).toBe(false);
  unmount();
  expect(eventSources[0].close).toHaveBeenCalledOnce();
});

test("URL restores request details on reload and preserves other parameters", async () => {
  history.replaceState(null, "", "/logs/?other=keep#anchor");
  const first = render(<RouterProvider router={createAppRouter()} />);
  fireEvent.click(await screen.findByRole("button", { name: /example.com/ }));
  await screen.findByRole("dialog", { name: "请求详情" });
  const selectedURL = location.href;
  first.unmount();
  render(<RouterProvider router={createAppRouter()} />);
  expect(
    await screen.findByRole("heading", { name: "GET · 200" }),
  ).toBeTruthy();
  expect(screen.getByRole("dialog", { name: "请求详情" })).toBeTruthy();
  expect(location.href).toBe(selectedURL);
  fireEvent.click(screen.getByRole("button", { name: "关闭详情" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(location.search).toBe("?other=keep");
  expect(location.hash).toBe("#anchor");
});

test("URL restores a request that is absent from the visible list", async () => {
  history.replaceState(null, "", "/logs/?request=outside%2Fpage");
  render(<RouterProvider router={createAppRouter()} />);
  expect(
    await screen.findByRole("heading", { name: "GET · 200" }),
  ).toBeTruthy();
  expect(screen.getByRole("dialog", { name: "请求详情" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "关闭详情" }));
  await waitFor(() => expect(location.search).toBe(""));
});

test("search sends the debounced query with a reset page offset", async () => {
  render(<RouterProvider router={createAppRouter()} />);
  await screen.findByRole("button", { name: /example.com/ });
  fireEvent.change(screen.getByRole("searchbox", { name: "搜索捕获记录" }), {
    target: { value: "example" },
  });
  await waitFor(() =>
    expect(
      fetchMock.mock.calls.some(
        ([path]) =>
          String(path).startsWith("/api/requests?") &&
          new URL(String(path), "http://localhost").searchParams.get("q") ===
            "example" &&
          new URL(String(path), "http://localhost").searchParams.get(
            "offset",
          ) === "0",
      ),
    ).toBe(true),
  );
});

test("caller column shows the client and preserves the full User-Agent", async () => {
  const userAgent =
    "claude-cli/2.1.295 (external, claude-desktop-3p, agent-sdk/0.3.295)";
  const original = fetchMock.getMockImplementation()!;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const response = await original(input);
      if (!String(input).startsWith("/api/requests?")) return response;
      const page = await response.json();
      page.items[0].metadata.user_agent = userAgent;
      Object.assign(page.items[0].metadata, {
        model: "claude-test",
        target_url: "https://example.com/api?stream=true",
        provider: "local",
        input_tokens: 0,
        output_tokens: 120,
        tokens_per_second: 60,
        routing_overhead_ms: 0,
        time_to_first_token_ms: 250,
        finish_reason: "end_turn",
      });
      return Response.json(page);
    }),
  );
  render(<RouterProvider router={createAppRouter()} />);
  expect(
    await screen.findByRole("cell", { name: "Claude CLI" }),
  ).toHaveProperty("title", userAgent);
  expect(screen.getByRole("columnheader", { name: "调用方" })).toBeTruthy();
  expect(
    screen.getAllByRole("columnheader").map((cell) => cell.textContent),
  ).toEqual([
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
  ]);
  const row = screen.getByRole("cell", { name: "Claude CLI" }).closest("tr")!;
  expect(
    within(row)
      .getAllByRole("cell")
      .slice(1)
      .map((cell) => cell.textContent),
  ).toEqual([
    "GET /api?stream=true",
    "200 · 已完成",
    "claude-test",
    "local",
    "Claude CLI",
    "0",
    "120",
    "10",
    "60",
    "0",
    "250",
    "—",
    "—",
    "end_turn",
  ]);
  expect(
    within(row).getByRole("button", {
      name: "GET https://example.com/api?stream=true",
    }),
  ).toHaveProperty("title", "GET https://example.com/api?stream=true");
});

test("unavailable inference metrics stay empty and request details remain accessible", async () => {
  render(<RouterProvider router={createAppRouter()} />);
  const button = await screen.findByRole("button", { name: /example.com/ });
  const row = button.closest("tr")!;
  expect(
    within(row)
      .getAllByRole("cell")
      .slice(6)
      .map((cell) => cell.textContent),
  ).toEqual([...Array(2).fill("—"), "10", ...Array(6).fill("—")]);
  fireEvent.click(button);
  expect(await screen.findByRole("dialog", { name: "请求详情" })).toBeTruthy();
});

test.each([
  ["unfinished", undefined, "— · 未完成", "pending"],
  ["unfinished", 200, "200 · 未完成", "pending"],
  ["error", 502, "502 · 传输中断", "failed"],
  ["completed", 404, "404 · 已完成", "failed"],
])(
  "status preserves lifecycle and HTTP result: %s / %s",
  async (state, code, label, className) => {
    const original = fetchMock.getMockImplementation()!;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const response = await original(input);
        if (!String(input).startsWith("/api/requests?")) return response;
        const page = await response.json();
        page.items[0].state = state;
        page.items[0].metadata.response_status_code = code;
        return Response.json(page);
      }),
    );
    render(<RouterProvider router={createAppRouter()} />);
    const cell = await screen.findByRole("cell", { name: label as string });
    expect(cell.classList.contains(className as string)).toBe(true);
    expect(
      within(cell.closest("tr")!).getAllByRole("cell")[8].textContent,
    ).toBe(state === "unfinished" ? "—" : "10");
  },
);

test("successful HTTP response remains distinct from capture failure", async () => {
  const original = fetchMock.getMockImplementation()!;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const response = await original(input);
      if (!String(input).startsWith("/api/requests?")) return response;
      const page = await response.json();
      page.items[0].state = "completed";
      page.items[0].capture_state = "failed";
      page.items[0].metadata.response_status_code = 200;
      return Response.json(page);
    }),
  );
  render(<RouterProvider router={createAppRouter()} />);
  const cell = await screen.findByRole("cell", {
    name: "200 · 采集失败",
  });
  expect(cell.classList.contains("failed")).toBe(true);
  expect(cell.title).toBe("200 · 已完成 · 采集失败");
});

test("interrupted response shows one status while preserving capture details", async () => {
  const original = fetchMock.getMockImplementation()!;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const response = await original(input);
      if (!String(input).startsWith("/api/requests?")) return response;
      const page = await response.json();
      page.items[0].state = "error";
      page.items[0].capture_state = "incomplete";
      return Response.json(page);
    }),
  );
  render(<RouterProvider router={createAppRouter()} />);
  const cell = await screen.findByRole("cell", { name: "200 · 传输中断" });
  expect(cell.title).toBe("200 · 请求异常 · 采集不完整");
  expect(cell.classList.contains("failed")).toBe(true);
});

test("request chart shows server totals and switches ranges with current filters", async () => {
  render(<RouterProvider router={createAppRouter()} />);
  expect(await screen.findByText("120 次请求 · 每 30 分钟")).toBeTruthy();
  expect(screen.getByRole("region", { name: "请求量趋势" })).toBeTruthy();
  fireEvent.change(screen.getByRole("combobox", { name: "捕获状态" }), {
    target: { value: "completed" },
  });
  fireEvent.click(screen.getByRole("radio", { name: "最近 7 天" }));
  await waitFor(() =>
    expect(
      fetchMock.mock.calls.some(([path]) =>
        String(path).includes("state=completed&hours=168"),
      ),
    ).toBe(true),
  );
});

test("home opens logs with the request log first in shared navigation", async () => {
  history.replaceState(null, "", "/");
  render(<RouterProvider router={createAppRouter()} />);
  await screen.findByRole("heading", { name: "请求日志", level: 1 });
  await screen.findByRole("link", { name: "常用配置" });
  const navigation = screen.getByRole("navigation", { name: "主导航" });
  expect(navigation.querySelector("a")?.textContent).toBe("请求日志");
  expect(
    screen.getByRole("link", { name: "请求日志" }).getAttribute("aria-current"),
  ).toBe("page");
  expect(
    screen.getByRole("link", { name: "TokenHub" }).getAttribute("href"),
  ).toBe("/");
});

test("logs header exposes accounts, providers and working restart controls", async () => {
  const original = fetchMock.getMockImplementation()!;
  const restart = vi.fn(() =>
    Response.json({
      state: "running",
      pending: false,
      active_provider: "local",
    }),
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === "/api/service/restart") {
        expect(init?.method).toBe("POST");
        expect(
          (init?.headers as Record<string, string>)["X-Tokenhub-Session"],
        ).toBe("session");
        return restart();
      }
      return original(input);
    }),
  );
  render(<RouterProvider router={createAppRouter()} />);
  await waitFor(() =>
    expect((screen.getByLabelText("当前账号") as HTMLSelectElement).value).toBe(
      "personal",
    ),
  );
  const header = screen.getByRole("banner");
  const pauseButton = screen.getByRole("button", { name: "暂停更新" });
  const examplesLink = screen.getByRole("link", { name: "请求示例" });
  expect(header.contains(pauseButton)).toBe(false);
  expect(header.contains(examplesLink)).toBe(false);
  expect(screen.getByRole("main").contains(pauseButton)).toBe(true);
  expect(
    screen.getByRole("navigation", { name: "主导航" }).contains(examplesLink),
  ).toBe(true);
  expect(examplesLink).toBe(
    screen.getByRole("navigation", { name: "主导航" }).lastElementChild,
  );
  expect(header.contains(screen.getByLabelText("当前账号"))).toBe(true);
  expect(header.contains(screen.getByLabelText("当前 Provider"))).toBe(true);
  const button = screen.getByRole("button", { name: "重启" });
  expect(header.contains(button)).toBe(true);
  await waitFor(() =>
    expect((button as HTMLButtonElement).disabled).toBe(false),
  );
  fireEvent.click(button);
  expect(await screen.findByRole("button", { name: "重启中…" })).toHaveProperty(
    "disabled",
    true,
  );
  await screen.findByText("服务已重启，已应用当前保存的配置。");
  expect(restart).toHaveBeenCalledTimes(1);
});

test("history pagination uses cursors and resets on search", async () => {
  const original = fetchMock.getMockImplementation()!;
  const historyFetch = vi.fn(async (input: RequestInfo | URL) => {
    const response = await original(input);
    if (!String(input).startsWith("/api/requests?")) return response;
    const page = await response.json();
    const params = new URL(String(input), "http://localhost").searchParams;
    page.total = 100000000;
    page.indexing = true;
    page.next_cursor = params.get("cursor") ? "third-page" : "second-page";
    return Response.json(page);
  });
  vi.stubGlobal("fetch", historyFetch);
  render(<RouterProvider router={createAppRouter()} />);
  expect(
    await screen.findByText("正在后台建立历史索引，当前展示已索引记录。"),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await waitFor(() =>
    expect(
      historyFetch.mock.calls.some(([url]) =>
        String(url).includes("cursor=second-page"),
      ),
    ).toBe(true),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "上一页" }).hasAttribute("disabled"),
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "上一页" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "上一页" }).hasAttribute("disabled"),
    ).toBe(true),
  );
  fireEvent.change(screen.getByRole("searchbox", { name: "搜索捕获记录" }), {
    target: { value: "new query" },
  });
  await waitFor(() =>
    expect(
      historyFetch.mock.calls.some(([url]) => {
        const p = new URL(String(url), "http://localhost").searchParams;
        return (
          p.get("q") === "new query" &&
          p.get("offset") === "0" &&
          p.get("cursor") === ""
        );
      }),
    ).toBe(true),
  );
});
