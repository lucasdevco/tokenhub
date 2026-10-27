// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { afterEach, expect, test, vi } from "vitest";
import { createAppRouter } from "../router";
import { buildCurl } from "./request-examples";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test.each(["chat", "responses", "claude"] as const)(
  "%s builds its protocol body and safely quotes input",
  (protocol) => {
    const code = buildCurl(
      protocol,
      "http://localhost:5601/test/v1/",
      "test-model",
      "What's $(whoami)?\n你好",
      true,
    );
    expect(code).toContain("curl --no-buffer 'http://localhost:5601/test/v1/");
    expect(code).toContain('"stream": true');
    expect(code).toContain("What'\"'\"'s $(whoami)?");
    expect(code).toContain(protocol === "responses" ? '"input"' : '"messages"');
    if (protocol === "claude") {
      expect(code).toContain('"max_tokens": 1024');
      expect(code).toContain("anthropic-version: 2023-06-01");
      expect(code).toContain("x-api-key:");
    } else expect(code).toContain("Authorization: Bearer");
  },
);

test("example route switches protocols, edits parameters and copies current curl", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input) => {
      const responses: Record<string, unknown> = {
        "/api/capabilities": { settings: true },
        "/api/session": { token: "session" },
        "/api/config": { yaml: "providers: {}", version: "v1" },
        "/api/status": { state: "running", pending: false },
      };
      return Response.json(responses[String(input)]);
    }),
  );
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  const router = createAppRouter(
    createMemoryHistory({ initialEntries: ["/examples"] }),
  );
  render(<RouterProvider router={router} />);
  await screen.findByRole("heading", { name: "请求示例" });
  fireEvent.click(screen.getByRole("radio", { name: "Claude Messages" }));
  expect(
    (screen.getByLabelText("API 基础地址") as HTMLInputElement).value,
  ).toBe("http://127.0.0.1:5601/v1");
  expect(
    screen.getByText("POST http://127.0.0.1:5601/v1/messages"),
  ).toBeTruthy();
  fireEvent.change(screen.getByLabelText("API 基础地址"), {
    target: { value: "http://localhost:5701/v1/" },
  });
  expect(
    screen.getByText("POST http://localhost:5701/v1/messages"),
  ).toBeTruthy();
  fireEvent.change(screen.getByLabelText("模型"), {
    target: { value: "my-model" },
  });
  fireEvent.click(screen.getByRole("button", { name: "复制 curl" }));
  await waitFor(() =>
    expect(writeText).toHaveBeenCalledWith(
      expect.stringContaining('"model": "my-model"'),
    ),
  );
  expect(writeText.mock.calls[0][0]).toContain(
    "http://localhost:5701/v1/messages",
  );
  writeText.mockRejectedValueOnce(Error("denied"));
  fireEvent.click(screen.getByRole("button", { name: "复制 curl" }));
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    "复制失败，请选中下方代码手动复制。",
  );
});
