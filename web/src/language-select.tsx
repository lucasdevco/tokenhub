import { useState } from "react";
import { isLocale, locales, setLocale, t, useLocale } from "./i18n";
import { NativeSelect } from "./components/ui/native-select";
import { request, setSession } from "./settings/settings-api";

export default function LanguageSelect() {
  const locale = useLocale();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <div className="language-select">
      <NativeSelect
        aria-label={t("Interface language")}
        value={locale}
        disabled={busy}
        onChange={async (event) => {
          const next = event.target.value;
          if (!isLocale(next)) return;
          setBusy(true);
          setError("");
          try {
            const response = await fetch("/api/preferences");
            // Standalone log viewers have no app preferences API.
            if (response.status !== 404) {
              if (!response.ok) throw Error(`HTTP ${response.status}`);
              const { preferences } = await response.json();
              setSession(
                (await request<{ token: string }>("/api/session")).token,
              );
              await request("/api/preferences", {
                ...preferences,
                locale: next,
              });
            }
            setLocale(next);
          } catch {
            setError(t("Could not save language settings."));
          } finally {
            setBusy(false);
          }
        }}
      >
        {locales.map(([code, name]) => (
          <option key={code} value={code}>
            {name}
          </option>
        ))}
      </NativeSelect>
      {error && <span role="alert">{error}</span>}
    </div>
  );
}
