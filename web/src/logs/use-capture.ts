import { useEffect, useRef, useState } from "react";
import { CaptureDecoder, fromBase64, hex, pretty } from "./decoders";
import { bytes, get, type RawPage } from "./api";

type Message = { id: number; label: string; text: string };
type Preview = { headers: string; text: string; messages: Message[] };
export type View = "stream" | "raw" | "hex";
const LIMIT = 2 * 1024 * 1024;

export function useCapture(
  name: string | undefined,
  view: View,
  following: boolean,
  rawOffset: number,
  revision: number,
  selection: number,
) {
  const [preview, setPreview] = useState<Preview>({
    headers: "",
    text: "",
    messages: [],
  });
  const [raw, setRaw] = useState<{ text: string; page: RawPage } | null>(null);
  const [status, setStatus] = useState("等待捕获文件");
  const [error, setError] = useState("");
  const current = useRef({
    offset: 0,
    preview: { headers: "", text: "", messages: [] } as Preview,
    decoder: null as CaptureDecoder | null,
    ended: false,
    id: 0,
  });
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = undefined;
    const data = {
      offset: 0,
      preview: { headers: "", text: "", messages: [] } as Preview,
      decoder: null as CaptureDecoder | null,
      ended: false,
      id: 0,
    };
    current.current = data;
    setPreview(data.preview);
    setRaw(null);
    setError("");
    setStatus(name ? "等待连接" : "等待捕获文件");
    const schedule = () => {
      if (timer.current) return;
      timer.current = setTimeout(() => {
        timer.current = undefined;
        if (current.current === data)
          setPreview({ ...data.preview, messages: [...data.preview.messages] });
      }, 80);
    };
    const text = (value: string) => {
      data.preview.text = (data.preview.text + value).slice(-LIMIT);
      schedule();
    };
    data.decoder = new CaptureDecoder(
      (label, value) => {
        const message = {
          id: ++data.id,
          label,
          text: pretty(value).slice(0, LIMIT),
        };
        const messages = data.preview.messages;
        messages.push(message);
        let size = messages.reduce((sum, item) => sum + item.text.length, 0);
        while (messages.length > 300 || size > LIMIT) {
          size -= messages.shift()!.text.length;
        }
        schedule();
      },
      (headers) => {
        data.preview.headers = headers;
        schedule();
      },
      text,
    );
    return () => {
      if (timer.current) clearTimeout(timer.current);
      timer.current = undefined;
    };
  }, [name, selection]);
  useEffect(() => {
    if (!name || view !== "stream") return;
    const data = current.current;
    if (data.ended) {
      setStatus(`已完成 · ${bytes(data.offset)}`);
      return;
    }
    if (!following) {
      setStatus(`预览已暂停 · ${bytes(data.offset)} · 磁盘捕获继续`);
      return;
    }
    let active = true;
    const stream = new EventSource(
      `/api/tail?name=${encodeURIComponent(name)}&offset=${data.offset}`,
    );
    setStatus("正在连接流");
    setError("");
    stream.onopen = () => {
      if (active) setStatus(`实时预览 · ${bytes(data.offset)}`);
    };
    stream.addEventListener("chunk", (event) => {
      if (!active) return;
      try {
        const chunk = JSON.parse(event.data);
        if (chunk.offset !== data.offset)
          throw Error("流偏移不一致，请重新选择该请求以重放。");
        data.decoder!.push(fromBase64(chunk.data));
        data.offset = chunk.next;
        setStatus(`实时预览 · ${bytes(data.offset)}`);
      } catch (value) {
        setError(String(value));
        stream.close();
      }
    });
    stream.addEventListener("end", () => {
      if (!active) return;
      data.decoder!.end();
      data.ended = true;
      setStatus(`已完成 · ${bytes(data.offset)}`);
      stream.close();
    });
    stream.addEventListener("waiting", () => {
      if (active) setStatus(`等待更多数据 · ${bytes(data.offset)}`);
    });
    stream.addEventListener("failure", () => {
      if (active) {
        setError("捕获文件读取失败，请重新选择或下载原文件。");
        stream.close();
      }
    });
    stream.onerror = () => {
      if (active) setStatus("正在重连 · 已接收数据保留");
    };
    return () => {
      active = false;
      stream.close();
    };
  }, [name, view, following, selection]);
  useEffect(() => {
    setRaw(null);
  }, [view, rawOffset]);
  useEffect(() => {
    if (!name || view === "stream") return;
    const controller = new AbortController();
    setError("");
    setStatus("正在读取原始数据");
    async function loadRaw() {
      if (view === "raw") {
        const response = await fetch(
          `/api/file?name=${encodeURIComponent(name!)}&download=1`,
          { signal: controller.signal },
        );
        if (!response.ok) throw Error(await response.text());
        const data = await response.arrayBuffer();
        if (controller.signal.aborted) return;
        setRaw({
          text: new TextDecoder().decode(data),
          page: {
            data: "",
            offset: 0,
            next: data.byteLength,
            size: data.byteLength,
          },
        });
        setStatus(`完整原始文件 · ${bytes(data.byteLength)}`);
        return;
      }
      const page = await get<RawPage>(
        `/api/file?name=${encodeURIComponent(name!)}&offset=${rawOffset}`,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      const data = fromBase64(page.data);
      const text = Array.from(
        { length: Math.ceil(data.length / 16) },
        (_, index) =>
          `${(page.offset + index * 16).toString(16).padStart(8, "0")}  ${hex(data.subarray(index * 16, index * 16 + 16))}`,
      ).join("\n");
      setRaw({ text, page });
      setStatus("原始文件 · 按 64 KiB 分页");
    }
    loadRaw().catch((value) => {
      if (!controller.signal.aborted) setError(String(value));
    });
    return () => controller.abort();
  }, [name, view, rawOffset, revision, selection]);
  return { preview, raw, status, error };
}
