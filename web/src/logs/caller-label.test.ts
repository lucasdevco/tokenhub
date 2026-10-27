import { expect, test } from "vitest";
import { callerLabel } from "./caller-label";

test("identifies callers from the primary User-Agent product", () => {
  expect(
    callerLabel(
      "claude-cli/2.1.295 (external, claude-desktop-3p, agent-sdk/0.3.295)",
    ),
  ).toBe("Claude CLI");
  expect(
    callerLabel(
      "deepseek-harness/0.2.0-rc.2 (+https://github.com/deepseek-ai/deepseek-harness)",
    ),
  ).toBe("DeepSeek Harness");
  expect(callerLabel("curl/8.7.1")).toBe("curl");
  expect(callerLabel("custom-client")).toBe("custom-client");
  expect(callerLabel("  CLAUDE-CLI/2.1.295  ")).toBe("Claude CLI");
  expect(callerLabel("   ")).toBe("—");
  expect(callerLabel()).toBe("—");
});
