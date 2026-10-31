import { expect, test } from "vitest";
import { checkDictionaries, sourceMessages } from "./check-i18n.mjs";

const catalog = {
  defaultLocale: "en",
  locales: { en: "English", ja: "日本語" },
};
const english = { "Hello {0}": "Hello {0}" };

test("checks a newly registered language without language-specific code", () => {
  expect(
    checkDictionaries(catalog, {
      en: english,
      ja: { "Hello {0}": "こんにちは {0}" },
    }),
  ).toEqual([]);
});

test("rejects missing dictionaries, missing translations and empty translations", () => {
  expect(checkDictionaries(catalog, { en: english })).toContain(
    "ja: dictionary is missing",
  );
  expect(checkDictionaries(catalog, { en: english, ja: {} })).toContain(
    'ja: missing "Hello {0}"',
  );
  expect(
    checkDictionaries(catalog, { en: english, ja: { "Hello {0}": " " } }),
  ).toContain('ja: empty or invalid "Hello {0}"');
});

test("rejects placeholder mismatches in both English and translated dictionaries", () => {
  expect(
    checkDictionaries(catalog, {
      en: english,
      ja: { "Hello {0}": "こんにちは {1}" },
    }),
  ).toContain('ja: placeholder mismatch "Hello {0}"');
  expect(
    checkDictionaries(catalog, {
      en: { "Hello {0}": "Hello" },
      ja: { "Hello {0}": "こんにちは" },
    }),
  ).toContain('en: placeholder mismatch "Hello {0}"');
});

test("rejects unregistered files and stale translation keys", () => {
  expect(
    checkDictionaries(catalog, {
      en: english,
      ja: { ...english, stale: "old" },
      fr: english,
    }),
  ).toEqual(
    expect.arrayContaining([
      "fr: dictionary is not registered",
      'ja: unknown key "stale"',
    ]),
  );
});

test("extracts static and dynamic source message keys from TypeScript and desktop code", () => {
  expect([
    ...sourceMessages(
      "page.tsx",
      't("Request logs"); const settingsItems = [["logs", "Request logs"]] as const; const labels = { running: "Running" }; setStatus({ message: "Loading…" }); t(`Save`); t(dynamic);',
    ),
  ]).toEqual(["Request logs", "Running", "Loading…", "Save"]);
  expect([
    ...sourceMessages(
      "panel.go",
      'tr("Request logs"); tr(`Settings`); tr(variable)',
    ),
  ]).toEqual(["Request logs", "Settings"]);
});

test("rejects Chinese translation keys", () => {
  expect(
    checkDictionaries(catalog, {
      en: { 请求日志: "Request logs" },
      ja: { 请求日志: "リクエストログ" },
    }),
  ).toContain('Translation keys must use English text: "请求日志"');
});
