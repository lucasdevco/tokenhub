// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { createAppRouter } from "./router";
import { getLocale, initializeLocale, setLocale, t } from "./i18n";
import { relativeTime } from "./logs/api";
import LanguageSelect from "./language-select";

afterEach(() => {
  cleanup();
  setLocale("en");
  localStorage.clear();
  vi.unstubAllGlobals();
});

test("defaults to English independently of the browser language", () => {
  expect(getLocale()).toBe("en");
  expect(t("Request logs")).toBe("Request logs");
  expect(t("Switched to {0}.", ["测试 {1}"])).toBe("Switched to 测试 {1}.");
  expect(relativeTime(new Date(Date.now() - 60000).toISOString())).toBe(
    "1 minute ago",
  );
});

test("switches the actual examples page without discarding the prompt", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("", { status: 404 })),
  );
  const router = createAppRouter(
    createMemoryHistory({ initialEntries: ["/examples"] }),
  );
  render(<RouterProvider router={router} />);
  await screen.findByRole("heading", { name: "Request examples" });
  fireEvent.change(screen.getByRole("textbox", { name: "Prompt" }), {
    target: { value: "Keep my draft" },
  });
  fireEvent.change(
    screen.getByRole("combobox", { name: "Interface language" }),
    { target: { value: "zh-CN" } },
  );
  await screen.findByRole("heading", { name: "请求示例" });
  expect(
    (screen.getByRole("textbox", { name: "提示词" }) as HTMLTextAreaElement)
      .value,
  ).toBe("Keep my draft");
  expect(document.documentElement.lang).toBe("zh-CN");
  expect(document.title).toBe("请求示例 · TokenHub");
  expect(localStorage.getItem("tokenhub.locale")).toBe("zh-CN");
  fireEvent.change(screen.getByRole("combobox", { name: "界面语言" }), {
    target: { value: "en" },
  });
  await screen.findByRole("heading", { name: "Request examples" });
  expect(document.documentElement.lang).toBe("en");
});

test("saves the language with existing desktop preferences and restores it", async () => {
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === "/api/session")
        return Response.json({ token: "session" });
      if (init?.method === "POST") return Response.json({});
      return Response.json({
        preferences: { auto_start: true, open_at_login: true, locale: "zh-CN" },
      });
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  await initializeLocale();
  expect(getLocale()).toBe("zh-CN");
  render(<LanguageSelect />);
  fireEvent.change(screen.getByRole("combobox", { name: "界面语言" }), {
    target: { value: "en" },
  });
  await waitFor(() => expect(getLocale()).toBe("en"));
  expect(fetchMock).toHaveBeenCalledWith(
    "/api/preferences",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({
        auto_start: true,
        open_at_login: true,
        locale: "en",
      }),
      headers: expect.objectContaining({ "X-Tokenhub-Session": "session" }),
    }),
  );
});

test("a failed save keeps the active language and shows an error", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("", { status: 500 })),
  );
  render(<LanguageSelect />);
  fireEvent.change(
    screen.getByRole("combobox", { name: "Interface language" }),
    { target: { value: "zh-CN" } },
  );
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    "Could not save language settings.",
  );
  expect(getLocale()).toBe("en");
});

test("legacy preferences and unsupported saved languages default to English", async () => {
  for (const preferences of [{}, { locale: "__unsupported__" }]) {
    setLocale("zh-CN");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ preferences })),
    );
    await initializeLocale();
    expect(getLocale()).toBe("en");
  }
});
