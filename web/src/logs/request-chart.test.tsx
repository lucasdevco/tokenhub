// @vitest-environment jsdom
import { beforeEach as beforeEachLocale } from "vitest";
import { setLocale } from "../i18n";
beforeEachLocale(() => setLocale("zh-CN"));
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import RequestChart from "./request-chart";

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    width: 800,
    height: 110,
    top: 0,
    left: 0,
    right: 800,
    bottom: 110,
    x: 0,
    y: 0,
    toJSON() {},
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const props = {
  histogram: {
    buckets: [{ started: "2026-10-14T10:00:00Z", count: 3 }],
    interval_seconds: 1800,
    total: 3,
  },
  hours: "24",
  onHoursChange: () => {},
  error: false,
};

test("library chart supports keyboard tooltips and preserves focus during updates", async () => {
  const { rerender } = render(<RequestChart {...props} loading={false} />);
  const chart = screen.getByRole("application", { name: "请求量趋势柱状图" });
  chart.focus();
  fireEvent.keyDown(chart, { key: "ArrowRight" });
  expect(await screen.findByText("3 次请求")).toBeTruthy();
  rerender(<RequestChart {...props} loading />);
  expect(document.activeElement).toBe(chart);
  rerender(
    <RequestChart
      {...props}
      histogram={{
        ...props.histogram,
        buckets: [{ ...props.histogram.buckets[0], count: 5 }],
        total: 5,
      }}
      loading={false}
    />,
  );
  await waitFor(() => expect(screen.getByText("5 次请求")).toBeTruthy());
  expect(document.activeElement).toBe(chart);
});

test("empty, loading and error states remain explicit", () => {
  const { rerender } = render(
    <RequestChart {...props} histogram={undefined} loading />,
  );
  expect(screen.getByText("正在加载请求量…")).toBeTruthy();
  rerender(
    <RequestChart
      {...props}
      histogram={{ ...props.histogram, total: 0, buckets: [] }}
      loading={false}
    />,
  );
  expect(screen.getByText("这段时间没有匹配的请求")).toBeTruthy();
  rerender(<RequestChart {...props} error loading={false} />);
  expect(screen.getByText("无法加载请求量，请刷新重试")).toBeTruthy();
});
