// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import CaptureDrawer from "./capture-drawer";

const storageKey = "tokenhub.logs.drawer-width-ratio";
const props = {
  requestKey: "request-1",
  detailLoading: true,
  detailError: "",
  revision: 0,
  selection: 0,
  onClose: () => {},
};

beforeEach(() => {
  localStorage.clear();
  vi.stubGlobal("innerWidth", 1000);
  vi.spyOn(HTMLDivElement.prototype, "getBoundingClientRect").mockReturnValue({
    width: 600,
  } as DOMRect);
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test("drawer defaults to 60% and keeps the last drag across remounts", () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const first = render(<CaptureDrawer {...props} />);
  const handle = screen.getByRole("separator", { name: "调整详情宽度" });
  Object.assign(handle, {
    setPointerCapture: vi.fn(),
    hasPointerCapture: () => false,
  });
  expect(handle.getAttribute("aria-valuenow")).toBe("600");
  fireEvent.pointerDown(handle, { button: 0, clientX: 400 });
  fireEvent.pointerMove(handle, { clientX: 300 });
  fireEvent.pointerUp(handle);
  expect(handle.getAttribute("aria-valuenow")).toBe("700");
  expect(localStorage.getItem(storageKey)).toBe("0.7");
  first.unmount();
  render(<CaptureDrawer {...props} />);
  expect(screen.getByRole("separator").getAttribute("aria-valuenow")).toBe(
    "700",
  );
  expect(screen.getByRole("dialog").style.width).toContain("70vw");
});

test("keyboard resizing is remembered and limited to viewport and minimum width", () => {
  render(<CaptureDrawer {...props} />);
  const handle = screen.getByRole("separator");
  fireEvent.keyDown(handle, { key: "ArrowLeft" });
  expect(localStorage.getItem(storageKey)).toBe("0.64");
  fireEvent.keyDown(handle, { key: "Home" });
  expect(handle.getAttribute("aria-valuenow")).toBe("360");
  fireEvent.keyDown(handle, { key: "End" });
  expect(handle.getAttribute("aria-valuenow")).toBe("1000");
});

test("invalid saved values fall back to 60% and narrow screens do not overflow", () => {
  localStorage.setItem(storageKey, "invalid");
  const first = render(<CaptureDrawer {...props} />);
  expect(screen.getByRole("separator").getAttribute("aria-valuenow")).toBe(
    "600",
  );
  first.unmount();
  vi.stubGlobal("innerWidth", 320);
  render(<CaptureDrawer {...props} />);
  expect(screen.getByRole("separator").getAttribute("aria-valuenow")).toBe(
    "320",
  );
});
