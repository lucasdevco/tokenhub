import { t, useLocale, type MessageKey } from "../i18n";
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
  useLocale();
  const [preview, setPreview] = useState<Preview>({
    headers: "",
    text: "",
    messages: [],
  });
  const [raw, setRaw] = useState<{ text: string; page: RawPage } | null>(null);
  const [status, setStatus] = useState<{
    message: MessageKey;
    values: readonly (string | number)[];
  }>({ message: "Waiting for capture file", values: [] });
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
    setStatus({
      message: name ? "Waiting for connection" : "Waiting for capture file",
      values: [],
    });
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
      setStatus({ message: "Completed · {0}", values: [bytes(data.offset)] });
      return;
    }
    if (!following) {
      setStatus({
        message: "Preview paused · {0} · Capture to disk continues",
        values: [bytes(data.offset)],
      });
      return;
    }
    let active = true;
    const stream = new EventSource(
      `/api/tail?name=${encodeURIComponent(name)}&offset=${data.offset}`,
    );
    setStatus({ message: "Connecting to stream", values: [] });
    setError("");
    stream.onopen = () => {
      if (active)
        setStatus({
          message: "Live preview · {0}",
          values: [bytes(data.offset)],
        });
    };
    stream.addEventListener("chunk", (event) => {
      if (!active) return;
      try {
        const chunk = JSON.parse(event.data);
        if (chunk.offset !== data.offset)
          throw Error(
            t("Stream offset mismatch. Select the request again to replay it."),
          );
        data.decoder!.push(fromBase64(chunk.data));
        data.offset = chunk.next;
        setStatus({
          message: "Live preview · {0}",
          values: [bytes(data.offset)],
        });
      } catch (value) {
        setError(String(value));
        stream.close();
      }
    });
    stream.addEventListener("end", () => {
      if (!active) return;
      data.decoder!.end();
      data.ended = true;
      setStatus({ message: "Completed · {0}", values: [bytes(data.offset)] });
      stream.close();
    });
    stream.addEventListener("waiting", () => {
      if (active)
        setStatus({
          message: "Waiting for more data · {0}",
          values: [bytes(data.offset)],
        });
    });
    stream.addEventListener("failure", () => {
      if (active) {
        setError(
          t(
            "Could not read the capture file. Select it again or download the original.",
          ),
        );
        stream.close();
      }
    });
    stream.onerror = () => {
      if (active)
        setStatus({
          message: "Reconnecting · Received data retained",
          values: [],
        });
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
    setStatus({ message: "Reading raw data", values: [] });
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
        setStatus({
          message: "Full raw file · {0}",
          values: [bytes(data.byteLength)],
        });
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
      setStatus({ message: "Raw file · 64 KiB pages", values: [] });
    }
    loadRaw().catch((value) => {
      if (!controller.signal.aborted) setError(String(value));
    });
    return () => controller.abort();
  }, [name, view, rawOffset, revision, selection]);
  return { preview, raw, status: t(status.message, status.values), error };
}
