export function callerLabel(userAgent?: string) {
  const product = userAgent?.trim().split(/[\s/]/, 1)[0];
  switch (product?.toLowerCase()) {
    case "claude-cli":
      return "Claude CLI";
    case "deepseek-harness":
      return "DeepSeek Harness";
    default:
      return product || "—";
  }
}
