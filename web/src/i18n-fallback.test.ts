import { afterEach, expect, test, vi } from "vitest";
import { isLocale, locales, setLocale, t } from "./i18n";

vi.mock("../../internal/i18n/locales/zh-CN.json", async (importOriginal) => {
  const original = await importOriginal<{ default: Record<string, string> }>();
  const messages = { ...original.default };
  delete messages["Request logs"];
  messages["Settings"] = "   ";
  return { default: messages };
});

afterEach(() => setLocale("en"));

test("missing and blank translations fall back to English while translated messages remain Chinese", () => {
  setLocale("zh-CN");
  expect(t("Request logs")).toBe("Request logs");
  expect(t("Settings")).toBe("Settings");
  expect(t("App preferences")).toBe("客户端偏好");
  expect(t("unknown external diagnostic")).toBe("unknown external diagnostic");
});

test("validates language selection against the shared registry", () => {
  for (const [code] of locales) expect(isLocale(code)).toBe(true);
  for (const value of ["__unsupported__", "toString", "__proto__", null, 5])
    expect(isLocale(value)).toBe(false);
});
