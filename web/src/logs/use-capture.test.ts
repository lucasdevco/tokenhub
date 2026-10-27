// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { useCapture, type View } from "./use-capture";

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  close = vi.fn();
  onopen?: () => void;
  onerror?: () => void;
  private listeners = new Map<string, (event: { data: string }) => void>();
  constructor(public url: string) {
    FakeEventSource.instances.push(this);
  }
  addEventListener(type: string, listener: (event: { data: string }) => void) {
    this.listeners.set(type, listener);
  }
  emit(type: string, value?: unknown) {
    this.listeners.get(type)?.({ data: JSON.stringify(value) });
  }
}

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

test("pause closes the stream and resume continues at the last byte offset", () => {
  vi.useFakeTimers();
  const { result, rerender, unmount } = renderHook(
    ({ following }) => useCapture("response.log", "stream", following, 0, 0, 0),
    { initialProps: { following: true } },
  );
  const stream = FakeEventSource.instances[0];
  const text = "HTTP/1.1 200 OK\r\n\r\nhello";
  act(() => {
    stream.emit("chunk", { offset: 0, next: text.length, data: btoa(text) });
    vi.advanceTimersByTime(80);
  });
  expect(result.current.preview.text).toBe("hello");
  rerender({ following: false });
  expect(stream.close).toHaveBeenCalledOnce();
  expect(result.current.status).toContain("预览已暂停");
  rerender({ following: true });
  const resumed = FakeEventSource.instances[1];
  expect(resumed.url).toContain(`offset=${text.length}`);
  unmount();
  expect(resumed.close).toHaveBeenCalledOnce();
});

test("switching files clears the preview and ignores events from the old stream", () => {
  vi.useFakeTimers();
  const { result, rerender } = renderHook(
    ({ name }) => useCapture(name, "stream", true, 0, 0, 0),
    { initialProps: { name: "first.log" } },
  );
  const first = FakeEventSource.instances[0];
  const text = "HTTP/1.1 200 OK\r\n\r\nold";
  act(() =>
    first.emit("chunk", { offset: 0, next: text.length, data: btoa(text) }),
  );
  rerender({ name: "second.log" });
  expect(first.close).toHaveBeenCalledOnce();
  expect(FakeEventSource.instances[1].url).toContain(
    "name=second.log&offset=0",
  );
  act(() => {
    first.emit("chunk", {
      offset: text.length,
      next: text.length + 3,
      data: btoa("old"),
    });
    vi.advanceTimersByTime(80);
  });
  expect(result.current.preview.text).toBe("");
});

test("invalid stream offsets stop decoding and report an error", () => {
  const { result } = renderHook(() =>
    useCapture("response.log", "stream", true, 0, 0, 0),
  );
  const stream = FakeEventSource.instances[0];
  act(() => stream.emit("chunk", { offset: 99, next: 100, data: btoa("x") }));
  expect(result.current.error).toContain("流偏移不一致");
  expect(stream.close).toHaveBeenCalledOnce();
});

test("changing the view aborts the old raw request and ignores its late response", async () => {
  let resolveFirst!: (response: Response) => void;
  const fetchMock = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          resolveFirst = resolve;
        }),
    )
    .mockResolvedValueOnce(
      Response.json({ data: btoa("ok"), offset: 0, next: 2, size: 2 }),
    );
  vi.stubGlobal("fetch", fetchMock);
  const { result, rerender } = renderHook(
    ({ view }: { view: View }) =>
      useCapture("response.log", view, true, 0, 0, 0),
    { initialProps: { view: "raw" as View } },
  );
  const signal = fetchMock.mock.calls[0][1].signal as AbortSignal;
  rerender({ view: "hex" });
  expect(signal.aborted).toBe(true);
  await waitFor(() => expect(result.current.raw?.text).toContain("6f 6b"));
  await act(async () => resolveFirst(new Response("stale raw content")));
  expect(result.current.raw?.text).not.toContain("stale");
  expect(result.current.status).toContain("64 KiB");
});
