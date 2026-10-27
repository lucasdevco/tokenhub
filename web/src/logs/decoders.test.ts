import { assert, test } from "vitest";
import { CaptureDecoder, SSEDecoder, WebSocketDecoder } from "./decoders";

test("SSE handles CR/LF boundaries, persistent IDs, multiline data and incomplete events", () => {
  const messages: [string, string][] = [];
  const decoder = new SSEDecoder((...event) => messages.push(event));
  for (const char of ": comment\r\nid: 42\r\nevent: delta\r\ndata: one\r\ndata: two\r\n\r\ndata: next\n\ndata: unfinished")
    decoder.push(char);
  assert.deepStrictEqual(messages, [
    ["delta · 42", "one\ntwo"],
    ["message · 42", "next"],
  ]);
});
test("capture reconstructs headers and UTF-8 across every byte boundary", () => {
  const events: [string, string][] = [],
    headers: string[] = [];
  const decoder = new CaptureDecoder(
    (...m) => events.push(m),
    (h) => headers.push(h),
    () => assert.fail("unexpected raw text"),
  );
  for (const byte of new TextEncoder().encode(
    "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream; charset=utf-8\r\n\r\ndata: €😀\n\n",
  ))
    decoder.push(Uint8Array.of(byte));
  decoder.end();
  assert.deepStrictEqual(events, [["message", "€😀"]]);
  assert.strictEqual(headers.length, 1);
});
test("WebSocket handles masked client frames, fragmentation and coalesced frames", () => {
  const events: [string, string][] = [];
  const decoder = new WebSocketDecoder((...m) => events.push(m));
  const frame = Uint8Array.of(
    0x81,
    0x82,
    1,
    2,
    3,
    4,
    "h".charCodeAt(0) ^ 1,
    "i".charCodeAt(0) ^ 2,
    0x89,
    0,
    0x82,
    2,
    0,
    255,
  );
  for (const b of frame) decoder.push(Uint8Array.of(b));
  assert.deepStrictEqual(events, [
    ["WebSocket · Text", "hi"],
    ["WebSocket · Ping", ""],
    ["WebSocket · Binary", "00 ff"],
  ]);
});
test("extended frame lengths and compressed frames retain binary representation", () => {
  const events: [string, string][] = [];
  const decoder = new WebSocketDecoder((...m) => events.push(m));
  decoder.push(Uint8Array.of(0x81, 126, 0, 3, 97, 98));
  decoder.push(Uint8Array.of(99, 0xc1, 2, 255, 0));
  assert.deepStrictEqual(events, [
    ["WebSocket · Text", "abc"],
    ["WebSocket · Text · compressed", "ff 00"],
  ]);
});
test("ordinary HTTP bodies are streamed without interpreting captured markup", () => {
  let text = "";
  const decoder = new CaptureDecoder(
    () => assert.fail(),
    () => {},
    (s) => (text += s),
  );
  decoder.push(
    new TextEncoder().encode("HTTP/1.1 200 OK\r\n\r\n<script>private</script>"),
  );
  decoder.end();
  assert.strictEqual(text, "<script>private</script>");
});
test("fragmented WebSocket text preserves split UTF-8 and compressed continuation flags", () => {
  const events: [string, string][] = [];
  const decoder = new WebSocketDecoder((...m) => events.push(m));
  decoder.push(
    Uint8Array.of(
      0x01,
      1,
      0xe2,
      0x89,
      0,
      0x80,
      2,
      0x82,
      0xac,
      0x41,
      1,
      0xff,
      0x80,
      1,
      0x00,
    ),
  );
  assert.deepStrictEqual(events, [
    ["WebSocket · Text · fragmented", ""],
    ["WebSocket · Ping", ""],
    ["WebSocket · Continuation", "€"],
    ["WebSocket · Text · fragmented · compressed", "ff"],
    ["WebSocket · Continuation · compressed", "00"],
  ]);
});
