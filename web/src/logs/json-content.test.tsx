// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import JsonContent from "./json-content";

afterEach(cleanup);

test("JSON containers can expand, collapse and reveal a long string", () => {
  const text = "a".repeat(300);
  render(
    <JsonContent text={JSON.stringify({ messages: [{ content: text }] })} />,
  );
  expect(screen.queryByText('"content":')).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "全部展开" }));
  expect(screen.getByText('"content":')).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "展开完整内容" }));
  expect(screen.getByText(JSON.stringify(text))).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "全部收起" }));
  expect(screen.queryByText('"content":')).toBeNull();
});

test("invalid JSON and captured markup render as inert text", () => {
  const text = '<script>window.secret = "exposed"</script>';
  const { container } = render(<JsonContent text={text} />);
  expect(container.querySelector("pre")?.textContent).toBe(text);
  expect(container.querySelector("script")).toBeNull();
});
