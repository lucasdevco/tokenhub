// @vitest-environment jsdom
import { beforeEach as beforeEachLocale } from "vitest";
import { setLocale } from "../i18n";
beforeEachLocale(() => setLocale("zh-CN"));
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import CapturePanel from "./capture-panel";
import { useCapture } from "./use-capture";

vi.mock("./use-capture", () => ({
  useCapture: vi.fn(() => ({
    preview: { headers: "", text: "", messages: [] },
    raw: {
      text:
        "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\n\r\n" +
        Array.from({ length: 12 }, (_, i) => `body line ${i + 1}`).join("\n"),
    },
    status: "已完成",
    error: "",
  })),
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test("view buttons switch modes and file view hides stream preview", () => {
  const { rerender } = render(
    <CapturePanel title="请求" revision={0} selection={0} />,
  );
  expect(screen.queryByRole("combobox")).toBeNull();
  expect(
    screen
      .getByRole("radio", { name: "原始文本" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("radio", { name: "十六进制" }));
  expect(vi.mocked(useCapture).mock.lastCall?.[1]).toBe("hex");
  expect(
    screen
      .getByRole("radio", { name: "十六进制" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(
    screen
      .getByRole("radio", { name: "原始文本" })
      .getAttribute("aria-checked"),
  ).toBe("false");
  fireEvent.click(screen.getByRole("radio", { name: "原始文本" }));
  expect(vi.mocked(useCapture).mock.lastCall?.[1]).toBe("raw");
  rerender(<CapturePanel title="原始文件" revision={0} selection={0} file />);
  expect(screen.queryByRole("button", { name: "实时预览 / JSON" })).toBeNull();
});

test.each(["", "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n"])(
  "empty response bodies show a hint while keeping original headers: %s",
  (text) => {
    vi.mocked(useCapture).mockReturnValueOnce({
      preview: { headers: "", text: "", messages: [] },
      raw: {
        text,
        page: { data: "", offset: 0, next: text.length, size: text.length },
      },
      status: "已完成",
      error: "",
    });
    const { container } = render(
      <CapturePanel
        title="响应"
        name="response.bin"
        revision={0}
        selection={0}
      />,
    );
    expect(screen.getByText("暂无响应体")).toBeTruthy();
    expect(container.querySelector("pre")?.textContent).toBe(text);
  },
);

test("response loading and errors do not claim the body is empty", () => {
  const capture = {
    preview: { headers: "", text: "", messages: [] },
    raw: null,
    status: "正在读取原始数据",
    error: "",
  };
  vi.mocked(useCapture).mockReturnValueOnce(capture);
  const { rerender } = render(
    <CapturePanel
      title="响应"
      name="response.bin"
      revision={0}
      selection={0}
    />,
  );
  expect(screen.getByText("正在加载响应体…")).toBeTruthy();
  expect(screen.queryByText("暂无响应体")).toBeNull();
  vi.mocked(useCapture).mockReturnValueOnce({ ...capture, error: "读取失败" });
  rerender(
    <CapturePanel
      title="响应"
      name="response.bin"
      revision={1}
      selection={0}
    />,
  );
  expect(screen.getByRole("alert").textContent).toBe("读取失败");
  expect(screen.queryByText("暂无响应体")).toBeNull();
  expect(screen.queryByText("正在加载响应体…")).toBeNull();
});

test("missing response capture shows an empty hint; received stream content hides it", () => {
  const { rerender } = render(
    <CapturePanel title="响应" revision={0} selection={0} />,
  );
  expect(screen.getByText("暂无响应体")).toBeTruthy();
  rerender(
    <CapturePanel
      title="响应"
      name="response.bin"
      revision={0}
      selection={0}
    />,
  );
  expect(screen.queryByText("暂无响应体")).toBeNull();
  fireEvent.click(screen.getByRole("radio", { name: "实时预览 / JSON" }));
  expect(screen.getByText("暂无响应体")).toBeTruthy();
  vi.mocked(useCapture).mockReturnValueOnce({
    preview: {
      headers: "HTTP/1.1 200 OK",
      text: "received data",
      messages: [],
    },
    raw: null,
    status: "实时预览",
    error: "",
  });
  rerender(
    <CapturePanel
      title="响应"
      name="response.bin"
      revision={1}
      selection={0}
    />,
  );
  expect(screen.queryByText("暂无响应体")).toBeNull();
  expect(screen.getByText("received data")).toBeTruthy();
});

test.each([
  [
    '{"messages":[{"role":"user","content":"你好"}],"stream":true}',
    '{\n  "messages": [\n    {\n      "role": "user",\n      "content": "你好"\n    }\n  ],\n  "stream": true\n}',
  ],
  ['[{"id":1}]', '[\n  {\n    "id": 1\n  }\n]'],
  ['{"incomplete":', '{"incomplete":'],
  ["plain text\n  original spacing", "plain text\n  original spacing"],
])(
  "raw request bodies are indented only for valid JSON: %s",
  (text, expected) => {
    vi.stubGlobal(
      "ResizeObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    const headers = "POST /api HTTP/1.1\r\nContent-Type: application/json";
    vi.mocked(useCapture).mockReturnValueOnce({
      preview: { headers: "", text: "", messages: [] },
      raw: {
        text: `${headers}\r\n\r\n${text}`,
        page: { data: "", offset: 0, next: 0, size: 0 },
      },
      status: "已完成",
      error: "",
    });
    const { container } = render(
      <CapturePanel title="请求" revision={0} selection={0} collapseBody />,
    );
    expect(
      container.querySelector(".capture-request-headers")?.textContent,
    ).toBe(headers);
    expect(container.querySelector(".capture-body pre")?.textContent).toBe(
      expected,
    );
  },
);

test.each([
  [
    '{"choices":[{"text":"你好"}]}',
    '{\n  "choices": [\n    {\n      "text": "你好"\n    }\n  ]\n}',
  ],
  ['{"incomplete":', '{"incomplete":'],
  [
    'data: {"text":"hello"}\n\ndata: [DONE]\n\n',
    'data: {"text":"hello"}\n\ndata: [DONE]\n\n',
  ],
])(
  "raw response bodies format JSON and preserve headers: %s",
  (text, expected) => {
    const headers = "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n";
    vi.mocked(useCapture).mockReturnValueOnce({
      preview: { headers: "", text: "", messages: [] },
      raw: {
        text: headers + text,
        page: { data: "", offset: 0, next: 0, size: 0 },
      },
      status: "已完成",
      error: "",
    });
    const { container } = render(
      <CapturePanel
        title="响应"
        name="response.bin"
        revision={0}
        selection={0}
      />,
    );
    expect(container.querySelector(".capture-body pre")?.textContent).toBe(
      headers + expected,
    );
  },
);

test.each(["请求", "响应"])(
  "long %s bodies toggle and reset on selection and resize",
  (title) => {
    let height = 420;
    const resizeCallbacks = new Set<() => void>();
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      () => ({ height }) as DOMRect,
    );
    const style = document.createElement("div").style;
    style.lineHeight = "20px";
    vi.stubGlobal("getComputedStyle", () => style);
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(private callback: () => void) {}
        observe() {
          resizeCallbacks.add(this.callback);
        }
        disconnect() {
          resizeCallbacks.delete(this.callback);
        }
      },
    );
    const { container, rerender } = render(
      <CapturePanel title={title} revision={0} selection={0} collapseBody />,
    );
    const toggle = screen.getByRole("button", {
      name: `展开完整${title} body（默认展示前 20 行）`,
    });
    const preview = container.querySelector(".capture-content-collapsed")!;
    expect(preview.textContent).toContain("body line 1");
    expect(preview.textContent).not.toContain("Host:");
    expect(
      container.querySelector(".capture-request-headers")?.textContent,
    ).toContain("Host: example.com");
    expect(
      preview.contains(container.querySelector(".capture-request-headers")),
    ).toBe(false);
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(container.querySelector(".capture-content-collapsed")).toBeTruthy();
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(toggle.textContent).toBe(`收起${title} body`);
    expect(container.querySelector(".capture-content-collapsed")).toBeNull();
    fireEvent.click(toggle);
    expect(container.querySelector(".capture-content-collapsed")).toBeTruthy();
    fireEvent.click(toggle);
    rerender(
      <CapturePanel title={title} revision={0} selection={1} collapseBody />,
    );
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    height = 400;
    act(() => resizeCallbacks.forEach((resize) => resize()));
    expect(screen.queryByRole("button", { name: /展开完整/ })).toBeNull();
    rerender(<CapturePanel title="响应" revision={0} selection={1} />);
    expect(screen.queryByRole("button", { name: /展开完整/ })).toBeNull();
    expect(container.querySelector(".capture-content-collapsed")).toBeNull();
  },
);
