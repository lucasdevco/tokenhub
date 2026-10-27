export function listStateLabel(state: string, captureState?: string) {
  if (state === "error") return "传输中断";
  if (captureState === "failed") return "采集失败";
  if (captureState === "incomplete") return "采集不完整";
  return stateLabel(state);
}

export function stateLabel(state: string, captureState?: string) {
  const result =
    state === "completed"
      ? "已完成"
      : state === "unfinished"
        ? "未完成"
        : "请求异常";
  if (captureState === "failed") return `${result} · 采集失败`;
  if (captureState === "incomplete") return `${result} · 采集不完整`;
  return result;
}
