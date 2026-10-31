import { t } from "../i18n";
export function listStateLabel(state: string, captureState?: string) {
  if (state === "error") return t("Transfer interrupted");
  if (captureState === "failed") return t("Capture failed");
  if (captureState === "incomplete") return t("Capture incomplete");
  return stateLabel(state);
}

export function stateLabel(state: string, captureState?: string) {
  const result =
    state === "completed"
      ? t("Completed")
      : state === "unfinished"
        ? t("unfinished")
        : t("Request error");
  if (captureState === "failed") return t("{0} · Capture failed", [result]);
  if (captureState === "incomplete")
    return t("{0} · Capture incomplete", [result]);
  return result;
}
