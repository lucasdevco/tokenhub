// @vitest-environment jsdom
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { createAppRouter } from "../router";

function renderSettings(path = "/settings") {
  const router = createAppRouter(
    createMemoryHistory({ initialEntries: [path] }),
  );
  render(<RouterProvider router={router} />);
  return router;
}

const yaml =
  "# retained\nserver:\n  host: 127.0.0.1\n  port: 5601\nproviders:\n  local:\n    base_url: https://example.com/\ncustom: preserved\n";
const status = {
  state: "running",
  pending: false,
  address: "127.0.0.1:5601",
  config_file: "config.yaml",
};
const initialCatalog = {
  services: [
    {
      id: "router",
      name: "OpenRouter",
      address: "https://openrouter.ai/api",
      kind: "openrouter",
      active: "personal",
      header: "Authorization",
      template: "Bearer {}",
      logging: true,
      proxy_enabled: true,
      accounts: [
        { id: "personal", name: "个人账号", key: "sk-test-personal" },
        { id: "work", name: "公司账号", key: "sk-test-work" },
      ],
    },
  ],
  active: "router",
  version: "catalog-v1",
  status,
};
let providerCatalog = structuredClone(initialCatalog);
let rejectSave = false;
const fetchMock = vi.fn(
  async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path === "/api/config" && init?.method === "POST") {
      return Response.json(
        rejectSave ? { error: "配置已被其他窗口修改" } : { version: "v2" },
        { status: rejectSave ? 409 : 200 },
      );
    }
    if (path === "/api/providers") {
      if (init?.method === "POST") {
        if (rejectSave)
          return Response.json(
            { error: "服务商已被其他窗口修改" },
            { status: 409 },
          );
        const change = JSON.parse(String(init.body));
        if (change.action === "save")
          providerCatalog.services = providerCatalog.services.map((service) =>
            service.id === change.service.id ? change.service : service,
          );
        if (change.action === "account")
          providerCatalog.services = providerCatalog.services.map((service) =>
            service.id === change.provider
              ? { ...service, active: change.account }
              : service,
          );
        providerCatalog.version = "catalog-v2";
      }
      return Response.json(providerCatalog);
    }
    const responses: Record<string, unknown> = {
      "/api/session": { token: "session" },
      "/api/config": { yaml, version: "v1", filename: "config.yaml" },
      "/api/status": status,
      "/api/preferences": {
        preferences: { auto_start: false, open_at_login: false },
        login_supported: false,
      },
      "/api/capabilities": { finder: false },
      "/api/config/validate": {},
    };
    if (!(path in responses)) throw Error(`Unexpected request: ${path}`);
    return Response.json(responses[path]);
  },
);

const defaultFetch = fetchMock.getMockImplementation()!;

beforeEach(() => {
  history.replaceState(null, "", "/settings");
  rejectSave = false;
  providerCatalog = structuredClone(initialCatalog);
  fetchMock.mockClear();
  fetchMock.mockImplementation(defaultFetch);
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test.each([
  ["重启", false],
  ["重启并应用", false],
  ["重启", true],
])(
  "%s keeps immediate restart feedback visible and confirms completion",
  async (label, failed) => {
    const originalFetch = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation(async (input, init) => {
      if (String(input) === "/api/status")
        return Response.json({ ...status, pending: true });
      if (String(input) === "/api/service/restart")
        return failed
          ? Response.json({ error: "端口已被占用" }, { status: 400 })
          : Response.json(status);
      return originalFetch(input, init);
    });
    renderSettings();
    await screen.findByRole("heading", { name: "常用配置" });
    vi.useFakeTimers();
    fireEvent.click(screen.getByRole("button", { name: label }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(599);
    });
    const buttons = screen.getAllByRole("button", { name: "重启中…" });
    expect(
      buttons.every((button) => (button as HTMLButtonElement).disabled),
    ).toBe(true);
    expect(screen.queryByText("服务已重启，已应用当前保存的配置。")).toBeNull();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    if (failed) {
      expect(screen.getByRole("alert").textContent).toBe("端口已被占用");
      expect(screen.queryByRole("status")).toBeNull();
    } else {
      expect(screen.getByRole("status").textContent).toBe(
        "服务已重启，已应用当前保存的配置。",
      );
    }
    expect(
      (screen.getByRole("button", { name: "重启" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
    expect(
      fetchMock.mock.calls.filter(([path]) => path === "/api/service/restart"),
    ).toHaveLength(1);
    fetchMock.mockImplementation(originalFetch);
  },
);

function openAdvanced() {
  const summary = screen.getByText("高级", { selector: "summary" });
  if (!summary.parentElement?.hasAttribute("open")) fireEvent.click(summary);
}

async function openYaml() {
  renderSettings();
  await screen.findByRole("heading", { name: "常用配置" });
  openAdvanced();
  fireEvent.click(screen.getByRole("link", { name: "完整 YAML" }));
  return screen.findByRole("textbox", { name: "完整 YAML 配置" });
}

test("route edits share the YAML draft and save its version, comments and unknown fields", async () => {
  renderSettings();
  await screen.findByRole("heading", { name: "常用配置" });
  fireEvent.change(screen.getByLabelText("监听端口"), {
    target: { value: "6000" },
  });
  openAdvanced();
  fireEvent.click(screen.getByRole("link", { name: "完整 YAML" }));
  await screen.findByRole("textbox", { name: "完整 YAML 配置" });
  const draft = (
    screen.getByRole("textbox", {
      name: "完整 YAML 配置",
    }) as HTMLTextAreaElement
  ).value;
  expect(draft).toContain("port: 6000");
  expect(draft).toContain("# retained");
  expect(draft).toContain("custom: preserved");
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText("配置已保存。重启代理后生效。");
  const saved = fetchMock.mock.calls.find(
    ([path, init]) => path === "/api/config" && init?.method === "POST",
  );
  expect(JSON.parse(saved![1]!.body as string)).toEqual({
    yaml: draft,
    version: "v1",
  });
  expect(saved![1]!.headers).toMatchObject({ "X-Tokenhub-Session": "session" });
  expect(
    (screen.getByRole("button", { name: "保存配置" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

test("a failed save keeps the edited draft available for retry", async () => {
  rejectSave = true;
  const editor = await openYaml();
  const draft = yaml + "other: retained\n";
  fireEvent.change(editor, { target: { value: draft } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText("配置已被其他窗口修改");
  expect((editor as HTMLTextAreaElement).value).toBe(draft);
  expect(
    (screen.getByRole("button", { name: "保存配置" }) as HTMLButtonElement)
      .disabled,
  ).toBe(false);
  expect(screen.getByText("有未保存的修改")).toBeTruthy();
});

test("invalid YAML prevents saving and reports the parse error", async () => {
  const editor = await openYaml();
  fireEvent.change(editor, { target: { value: "server: *missing" } });
  expect(
    (screen.getByRole("button", { name: "保存配置" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.getByRole("alert").textContent).toMatch(/alias/i);
});

test("settings no longer expose or request environment configuration", async () => {
  renderSettings();
  await screen.findByRole("heading", { name: "常用配置" });
  openAdvanced();
  expect(screen.queryByRole("link", { name: /\.env/ })).toBeNull();
  expect(screen.queryByLabelText("当前环境")).toBeNull();
  expect(screen.getByLabelText("当前账号")).toBeTruthy();
  expect(
    fetchMock.mock.calls.some(([input]) =>
      String(input).startsWith("/api/environment"),
    ),
  ).toBe(false);
});

test("service and account changes persist with a management session and version", async () => {
  const router = renderSettings("/settings/providers");
  await screen.findByRole("heading", { name: "服务商" });
  expect(router.state.location.searchStr).toBe("");
  expect(screen.queryByText("交互预览")).toBeNull();
  expect(
    screen
      .getByRole("link", { name: "完整 YAML", hidden: true })
      .closest("details")
      ?.hasAttribute("open"),
  ).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "编辑 OpenRouter" }));
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "我的服务" },
  });
  fireEvent.click(screen.getByText("高级设置", { selector: "summary" }));
  const proxyToggle = screen.getByRole("switch", { name: "启用上游代理" });
  expect(proxyToggle.getAttribute("aria-checked")).toBe("true");
  fireEvent.click(proxyToggle);
  fireEvent.click(screen.getByRole("button", { name: "添加账号" }));
  fireEvent.change(screen.getByLabelText("账号名称"), {
    target: { value: "备用账号" },
  });
  fireEvent.change(screen.getByLabelText("API Key"), {
    target: { value: "sk-demo-backup" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  expect(
    await screen.findByRole("button", { name: "编辑 我的服务" }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "切换账号" }));
  fireEvent.click(screen.getByRole("menuitemradio", { name: "备用账号" }));
  await screen.findByText("已切换到 备用账号，新请求立即生效。");
  const mutations = fetchMock.mock.calls.filter(
    ([path, init]) => path === "/api/providers" && init?.method === "POST",
  );
  expect(mutations).toHaveLength(2);
  expect(JSON.parse(String(mutations[0][1]?.body))).toMatchObject({
    action: "save",
    version: "catalog-v1",
    service: { name: "我的服务", proxy_enabled: false },
  });
  expect(JSON.parse(String(mutations[1][1]?.body))).toMatchObject({
    action: "account",
    provider: "router",
    version: "catalog-v2",
  });
  expect(mutations[0][1]?.headers).toMatchObject({
    "X-Tokenhub-Session": "session",
  });
});

test("provider save keeps the inline form mounted and clears the dirty draft", async () => {
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
  renderSettings("/settings/providers");
  fireEvent.click(
    await screen.findByRole("button", { name: "编辑 OpenRouter" }),
  );
  const name = screen.getByLabelText("名称");
  const save = screen.getByRole("button", { name: "保存" });
  fireEvent.change(name, { target: { value: " 我的服务 " } });
  const advanced = screen
    .getByText("高级设置", { selector: "summary" })
    .closest("details")!;
  fireEvent.click(screen.getByText("高级设置", { selector: "summary" }));
  fireEvent.click(save);
  await screen.findByText("已保存 我的服务，新请求将使用保存后的设置。");
  expect(screen.getByLabelText("名称")).toBe(name);
  expect(name).toHaveProperty("value", "我的服务");
  expect(screen.getByRole("button", { name: "保存" })).toBe(save);
  expect(advanced.open).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "编辑 我的服务" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "关闭编辑" }));
  expect(confirm).not.toHaveBeenCalled();
});

test("service edit cancel keeps saved values and protects an unfinished draft", async () => {
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
  renderSettings("/settings/providers");
  fireEvent.click(
    await screen.findByRole("button", { name: "编辑 OpenRouter" }),
  );
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "未保存名称" },
  });
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(confirm).toHaveBeenCalledOnce();
  expect(screen.getByLabelText("名称")).toHaveProperty("value", "未保存名称");
  confirm.mockReturnValue(true);
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.getByRole("button", { name: "编辑 OpenRouter" })).toBeTruthy();
});

test("failed provider saves retain the open draft for retry", async () => {
  rejectSave = true;
  renderSettings("/settings/providers");
  fireEvent.click(
    await screen.findByRole("button", { name: "编辑 OpenRouter" }),
  );
  fireEvent.change(screen.getByLabelText("名称"), {
    target: { value: "未保存名称" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await screen.findByText("服务商已被其他窗口修改");
  expect(screen.getByLabelText("名称")).toHaveProperty("value", "未保存名称");
  expect(
    (screen.getByRole("button", { name: "保存" }) as HTMLButtonElement)
      .disabled,
  ).toBe(false);
});

test("failed account switches keep the current account", async () => {
  rejectSave = true;
  renderSettings("/settings/providers");
  fireEvent.click(await screen.findByRole("button", { name: "切换账号" }));
  fireEvent.click(screen.getByRole("menuitemradio", { name: "公司账号" }));
  await screen.findByText("服务商已被其他窗口修改");
  expect(
    screen.getByRole("button", { name: "切换账号" }).textContent,
  ).toContain("个人账号");
});

test.each([
  ["basic", "常用配置"],
  ["providers", "服务商"],
  ["yaml", "完整 YAML"],
  ["app", "客户端偏好"],
])(
  "direct /settings/%s opens the matching page and navigation",
  async (path, heading) => {
    renderSettings(`/settings/${path}`);
    await screen.findByRole("heading", { name: heading });
    expect(
      screen.getByRole("link", { name: heading }).getAttribute("aria-current"),
    ).toBe("page");
    expect(
      screen.getByRole("link", { name: "请求日志" }).getAttribute("href"),
    ).toBe("/logs");
  },
);

test("back and forward restore the setting and keep its unsaved draft", async () => {
  const router = renderSettings();
  await screen.findByRole("heading", { name: "常用配置" });
  expect(router.state.location.pathname).toBe("/settings/basic");
  openAdvanced();
  fireEvent.click(screen.getByRole("link", { name: "完整 YAML" }));
  const editor = await screen.findByRole("textbox", { name: "完整 YAML 配置" });
  const draft = yaml + "other: retained\n";
  fireEvent.change(editor, { target: { value: draft } });
  fireEvent.click(screen.getByRole("link", { name: "常用配置" }));
  await screen.findByRole("heading", { name: "常用配置" });
  act(() => router.history.back());
  expect(
    (
      (await screen.findByRole("textbox", {
        name: "完整 YAML 配置",
      })) as HTMLTextAreaElement
    ).value,
  ).toBe(draft);
  act(() => router.history.forward());
  await screen.findByRole("heading", { name: "常用配置" });
  expect(
    screen.getByRole("link", { name: "常用配置" }).getAttribute("aria-current"),
  ).toBe("page");
});

test("leaving settings for logs protects unsaved config", async () => {
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
  const router = renderSettings("/settings/yaml");
  const editor = await screen.findByRole("textbox", { name: "完整 YAML 配置" });
  fireEvent.change(editor, { target: { value: yaml + "other: retained\n" } });
  fireEvent.click(screen.getByRole("link", { name: "请求日志" }));
  await waitFor(() => expect(confirm).toHaveBeenCalledOnce());
  expect(router.state.location.pathname).toBe("/settings/yaml");
  expect((editor as HTMLTextAreaElement).value).toContain("other: retained");
});

test("global provider selection persists immediately without restarting or changing URL", async () => {
  const original = fetchMock.getMockImplementation()!;
  let active = "";
  fetchMock.mockImplementation(async (input, init) => {
    if (String(input) === "/api/status")
      return Response.json({ ...status, active_provider: active });
    if (String(input) === "/api/provider") {
      active = JSON.parse(String(init?.body)).provider;
      return Response.json({
        yaml: yaml + `active_provider: ${active}\n`,
        version: "v2",
        filename: "config.yaml",
      });
    }
    return original(input, init);
  });
  const router = renderSettings("/settings/basic");
  await screen.findByRole("heading", { name: "常用配置" });
  const control = screen.getByLabelText("当前 Provider") as HTMLSelectElement;
  fireEvent.change(control, { target: { value: "local" } });
  await screen.findByText("已切换到 local。");
  expect(control.value).toBe("local");
  expect(router.state.location.pathname).toBe("/settings/basic");
  const mutation = fetchMock.mock.calls.find(
    ([input]) => String(input) === "/api/provider",
  );
  expect(JSON.parse(String(mutation?.[1]?.body))).toEqual({
    provider: "local",
    version: "v1",
  });
  expect(
    fetchMock.mock.calls.some(([input]) =>
      String(input).includes("/api/service/"),
    ),
  ).toBe(false);
});

test("provider selection failure retains the current provider and YAML draft", async () => {
  const original = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (input, init) => {
    if (String(input) === "/api/provider")
      return Response.json(
        { error: "配置已被修改，请重新加载后再切换" },
        { status: 400 },
      );
    return original(input, init);
  });
  renderSettings("/settings/basic");
  await screen.findByRole("heading", { name: "常用配置" });
  fireEvent.change(screen.getByLabelText("当前 Provider"), {
    target: { value: "local" },
  });
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    "配置已被修改，请重新加载后再切换",
  );
  expect(
    (screen.getByLabelText("当前 Provider") as HTMLSelectElement).value,
  ).toBe("");
  expect((screen.getByLabelText("监听端口") as HTMLInputElement).value).toBe(
    "5601",
  );
});

test("unsaved configuration edits prevent switching", async () => {
  renderSettings("/settings/basic");
  await screen.findByRole("heading", { name: "常用配置" });
  fireEvent.change(screen.getByLabelText("监听端口"), {
    target: { value: "6000" },
  });
  expect(
    (screen.getByLabelText("当前 Provider") as HTMLSelectElement).disabled,
  ).toBe(true);
  expect(
    screen.getByLabelText("当前 Provider").getAttribute("title"),
  ).toBeTruthy();
});

test.each([false, true])(
  "topbar account switch persists without restart; conflict=%s",
  async (conflict) => {
    const original = fetchMock.getMockImplementation()!;
    let active = "personal";
    const accountConfig = () => yaml + `active_provider: local\n`;
    fetchMock.mockImplementation(async (input, init) => {
      if (String(input) === "/api/config" && !init?.method?.includes("POST")) {
        return Response.json({
          yaml: accountConfig().replace(
            "    base_url: https://example.com/",
            `    base_url: https://example.com/\n    active_account: ${active}\n    accounts: [{id: personal, name: 个人, key: test-personal}, {id: work, name: 公司, key: test-work}]`,
          ),
          version: "v1",
          filename: "config.yaml",
        });
      }
      if (String(input) === "/api/status")
        return Response.json({ ...status, active_provider: "local" });
      if (String(input) === "/api/providers" && init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({
          action: "account",
          provider: "local",
          account: "work",
          version: "v1",
        });
        if (conflict)
          return Response.json(
            { error: "配置已被修改，请重新加载" },
            { status: 409 },
          );
        active = "work";
        return Response.json({
          status: { ...status, active_provider: "local" },
        });
      }
      return original(input, init);
    });
    renderSettings();
    await screen.findByRole("heading", { name: "常用配置" });
    const control = screen.getByLabelText("当前账号") as HTMLSelectElement;
    expect(control.value).toBe("personal");
    fireEvent.change(control, { target: { value: "work" } });
    await screen.findByText(
      conflict ? "配置已被修改，请重新加载" : "账号已切换，新请求立即生效。",
    );
    expect(control.value).toBe(conflict ? "personal" : "work");
    expect(
      fetchMock.mock.calls.some(([input]) =>
        String(input).startsWith("/api/service/"),
      ),
    ).toBe(false);
    fireEvent.change(screen.getByLabelText("监听端口"), {
      target: { value: "6000" },
    });
    expect(control.disabled).toBe(true);
  },
);
