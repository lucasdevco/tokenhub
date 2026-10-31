import { useSyncExternalStore } from "react";
import catalog from "../../internal/i18n/catalog.json";
import type english from "../../internal/i18n/locales/en.json";

export type MessageKey = keyof typeof english;

export type Locale = keyof typeof catalog.locales;
export const locales = Object.entries(catalog.locales);
export function isLocale(value: unknown): value is Locale {
  return typeof value === "string" && Object.hasOwn(catalog.locales, value);
}
export const defaultLocale: Locale = isLocale(catalog.defaultLocale)
  ? catalog.defaultLocale
  : (() => {
      throw Error("Invalid default locale");
    })();
const dictionaries = import.meta.glob<Record<string, string>>(
  "../../internal/i18n/locales/*.json",
  { eager: true, import: "default" },
);
function lookupMessage(locale: Locale, key: string) {
  const dictionary = dictionaries[`../../internal/i18n/locales/${locale}.json`];
  const translated =
    dictionary && Object.hasOwn(dictionary, key) ? dictionary[key] : undefined;
  if (translated?.trim()) return translated;
  if (locale !== defaultLocale) return lookupMessage(defaultLocale, key);
  return key;
}
const listeners = new Set<() => void>();
const storageKey = "tokenhub.locale";
function storedLocale(): Locale {
  try {
    const saved = localStorage.getItem(storageKey);
    return isLocale(saved) ? saved : defaultLocale;
  } catch {
    return defaultLocale;
  }
}
let locale: Locale = storedLocale();
export const getLocale = () => locale;
export function setLocale(value: Locale) {
  locale = value;
  if (typeof document !== "undefined") document.documentElement.lang = value;
  try {
    localStorage.setItem(storageKey, value);
  } catch {
    // Language selection still works when browser storage is unavailable.
  }
  listeners.forEach((listener) => listener());
}
export function useLocale() {
  return useSyncExternalStore((listener) => {
    listeners.add(listener);
    return () => listeners.delete(listener);
  }, getLocale);
}

// Prefer the selected dictionary, then English. Preserve unknown external diagnostics.
export function t(message: string, values: readonly (string | number)[] = []) {
  const template = lookupMessage(locale, message);
  return template.replace(/\{(\d+)\}/g, (match, index: string) =>
    values[Number(index)] === undefined ? match : String(values[Number(index)]),
  );
}

export async function initializeLocale() {
  const initial = locale;
  document.documentElement.lang = locale;
  try {
    const response = await fetch("/api/preferences");
    if (!response.ok) return;
    const result = await response.json();
    if (locale === initial)
      setLocale(
        isLocale(result.preferences?.locale)
          ? result.preferences.locale
          : defaultLocale,
      );
  } catch {
    // Log-only servers and offline clients use the saved browser preference.
  }
}
