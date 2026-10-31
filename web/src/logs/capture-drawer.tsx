import { t, useLocale } from "../i18n";
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetDescription,
} from "../components/ui/sheet";
import { Alert, AlertDescription } from "../components/ui/alert";
import { useRef, useState } from "react";
import {
  bytes,
  downloadURL,
  timestamp,
  type CaptureFile,
  type Transaction,
} from "./api";
import CapturePanel from "./capture-panel";
import { stateLabel } from "./state-label";

type Props = {
  requestKey?: string;
  detail?: Transaction;
  file?: CaptureFile;
  detailLoading: boolean;
  detailError: string;
  revision: number;
  selection: number;
  onClose: () => void;
};

const WIDTH_STORAGE_KEY = "tokenhub.logs.drawer-width-ratio";

export default function CaptureDrawer({
  requestKey,
  detail,
  file,
  detailLoading,
  detailError,
  revision,
  selection,
  onClose,
}: Props) {
  useLocale();
  const drawer = useRef<HTMLDivElement>(null);
  const [drawerRatio, setDrawerRatio] = useState(() => {
    try {
      const saved = Number(localStorage.getItem(WIDTH_STORAGE_KEY));
      if (saved > 0 && saved <= 1) return saved;
    } catch {}
    return 0.6;
  });
  const drawerWidth = Math.min(
    window.innerWidth,
    Math.max(360, window.innerWidth * drawerRatio),
  );
  const resizeStart = useRef<{ x: number; width: number } | null>(null);
  function resizeDrawer(width: number) {
    const ratio =
      Math.min(window.innerWidth, Math.max(360, width)) / window.innerWidth;
    setDrawerRatio(ratio);
    try {
      localStorage.setItem(WIDTH_STORAGE_KEY, String(ratio));
    } catch {}
  }
  const drawerOpen = !!requestKey || !!file;
  return (
    <Sheet
      open={drawerOpen}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent
        ref={drawer}
        className="logs-drawer"
        style={{ width: `min(100vw, max(360px, ${drawerRatio * 100}vw))` }}
        aria-label={t("Request and response details")}
      >
        <div
          className="drawer-resize-handle"
          role="separator"
          aria-label={t("Resize details")}
          aria-orientation="vertical"
          aria-valuemin={Math.min(360, window.innerWidth)}
          aria-valuemax={window.innerWidth}
          aria-valuenow={Math.min(drawerWidth, window.innerWidth)}
          tabIndex={0}
          onPointerDown={(event) => {
            if (event.button !== 0) return;
            event.preventDefault();
            event.currentTarget.focus();
            event.currentTarget.setPointerCapture(event.pointerId);
            resizeStart.current = {
              x: event.clientX,
              width: drawer.current!.getBoundingClientRect().width,
            };
          }}
          onPointerMove={(event) => {
            const start = resizeStart.current;
            if (start) resizeDrawer(start.width + start.x - event.clientX);
          }}
          onPointerUp={(event) => {
            resizeStart.current = null;
            if (event.currentTarget.hasPointerCapture(event.pointerId))
              event.currentTarget.releasePointerCapture(event.pointerId);
          }}
          onLostPointerCapture={() => {
            resizeStart.current = null;
          }}
          onKeyDown={(event) => {
            const width = drawer.current!.getBoundingClientRect().width;
            if (event.key === "ArrowLeft") resizeDrawer(width + 40);
            else if (event.key === "ArrowRight") resizeDrawer(width - 40);
            else if (event.key === "Home") resizeDrawer(360);
            else if (event.key === "End") resizeDrawer(window.innerWidth);
            else return;
            event.preventDefault();
          }}
        />
        <div className="drawer-body">
          <div className="drawer-header">
            <SheetTitle>{t("Request details")}</SheetTitle>
            <SheetDescription className="sr-only">
              {t(
                "Inspect headers, response content, SSE events, and WebSocket frames.",
              )}
            </SheetDescription>
          </div>
          <section
            className="capture-inspector"
            aria-label={t("Request details")}
          >
            {!detail && !file ? (
              <div className="inspector-empty">
                <h2>
                  {detailLoading ? t("Loading request…") : t("Select a record")}
                </h2>
                <p>
                  {detailError ||
                    t(
                      "Inspect headers, response content, SSE events, and WebSocket frames.",
                    )}
                </p>
              </div>
            ) : (
              <>
                <div className="detail-heading">
                  <h2>
                    {file
                      ? t("Capture file")
                      : `${detail?.metadata.method || t("Request")} · ${detail?.metadata.response_status_code || t("Waiting for response")}`}
                  </h2>
                  <p className="detail-url">
                    {file?.name ||
                      detail?.metadata.target_url ||
                      detail?.metadata.source_url}
                  </p>
                  <p className="logs-footnote">
                    {file
                      ? `${timestamp(file.modified)} · ${bytes(file.size)}`
                      : `${timestamp(detail!.started)} · ${detail!.duration_ms} ms · ${stateLabel(detail!.state, detail!.capture_state)}`}
                    {detail?.metadata.provider
                      ? ` · Provider: ${detail.metadata.provider}`
                      : ""}
                    {detail?.capture_wait_ms !== undefined
                      ? t(" · Capture wait {0} ms", [
                          detail.capture_wait_ms.toFixed(2),
                        ])
                      : ""}
                    {detailLoading ? t(" · Updating") : ""}
                  </p>
                </div>
                {detail && (
                  <details className="capture-metadata">
                    <summary>{t("Lifecycle and metadata")}</summary>
                    <div className="metadata-downloads">
                      {detail.metadata_files?.map((filename) => (
                        <a
                          key={filename}
                          href={downloadURL(filename)}
                          download={filename}
                        >
                          {filename}
                        </a>
                      ))}
                    </div>
                    <pre>{JSON.stringify(detail, null, 2)}</pre>
                  </details>
                )}
                {(detailError || detail?.problems?.length || file?.problem) && (
                  <Alert
                    variant="destructive"
                    className="message error"
                    role="status"
                  >
                    <AlertDescription>
                      {detailError ||
                        detail?.problems?.join("\n") ||
                        file?.problem}
                    </AlertDescription>
                  </Alert>
                )}
                {detail ? (
                  <>
                    <CapturePanel
                      key={`${requestKey}:request:${selection}`}
                      title={t("Request")}
                      collapseBody
                      name={detail.request_file}
                      revision={revision}
                      selection={selection}
                    />
                    <CapturePanel
                      key={`${requestKey}:response:${selection}`}
                      title={t("Response")}
                      collapseBody
                      name={detail.response_file}
                      revision={revision}
                      selection={selection}
                    />
                  </>
                ) : file ? (
                  <CapturePanel
                    key={`${file.name}:${selection}`}
                    title={t("Raw file")}
                    name={file.name}
                    revision={revision}
                    selection={selection}
                    file
                  />
                ) : null}
              </>
            )}
          </section>
        </div>
      </SheetContent>
    </Sheet>
  );
}
