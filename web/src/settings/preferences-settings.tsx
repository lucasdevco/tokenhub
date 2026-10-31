import { getLocale, t, useLocale, type Locale } from "../i18n";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { request as api } from "./settings-api";
import { Field, Toggle } from "./settings-controls";

export type Preferences = {
  locale?: Locale;
  auto_start: boolean;
  open_at_login: boolean;
};
type Props = {
  prefs?: Preferences;
  setPrefs: (preferences: Preferences) => void;
  filename: string;
  locked: boolean;
  loginSupported: boolean;
  finderSupported: boolean;
  setError: (error: string) => void;
  setNotice: (notice: string) => void;
  setDraft: (draft: string) => void;
  onOpenYaml: () => void;
  perform: (action: () => Promise<void>) => Promise<void>;
};

export default function PreferencesSettings({
  prefs,
  setPrefs,
  filename,
  locked,
  loginSupported,
  finderSupported,
  setError,
  setNotice,
  setDraft,
  onOpenYaml,
  perform,
}: Props) {
  useLocale();
  return (
    <section>
      <h1>{t("App preferences")}</h1>
      <p className="intro">
        {t(
          "Preferences are saved separately and do not require a proxy restart.",
        )}
      </p>
      {prefs && (
        <>
          <Field label={t("Start service automatically")}>
            <Toggle
              label={t("Start the proxy when the app opens")}
              disabled={locked}
              value={prefs.auto_start}
              onChange={(value) =>
                perform(async () => {
                  const next = {
                    ...prefs,
                    locale: getLocale(),
                    auto_start: value,
                  };
                  await api("/api/preferences", next);
                  setPrefs(next);
                  setNotice(t("Preferences saved."));
                })
              }
            />
          </Field>
          <Field
            label={t("Open at login")}
            help={
              loginSupported
                ? t("Requires the packaged macOS app to be installed.")
                : t("Available in the macOS menu bar client.")
            }
          >
            <Toggle
              label={t("Open TokenHub when you log in to macOS")}
              disabled={locked || !loginSupported}
              value={prefs.open_at_login}
              onChange={(value) =>
                perform(async () => {
                  const next = {
                    ...prefs,
                    locale: getLocale(),
                    open_at_login: value,
                  };
                  await api("/api/preferences", next);
                  setPrefs(next);
                  setNotice(t("Login settings updated."));
                })
              }
            />
          </Field>
        </>
      )}
      <h2>{t("Configuration file")}</h2>
      <code className="config-path">{filename}</code>
      <Button
        variant="outline"
        size="sm"
        className="open-config-directory"
        disabled={locked || !finderSupported}
        title={
          finderSupported
            ? t("Open the configuration directory")
            : t("Requires the macOS desktop client")
        }
        onClick={() =>
          perform(async () => {
            await api("/api/directories/config/open", {});
          })
        }
      >
        Open in Finder
      </Button>
      <h2>{t("Import configuration")}</h2>
      <p className="hint">
        {t(
          "Import replaces only the current draft. It is validated before saving. Relative paths use the current configuration directory.",
        )}
      </p>
      <Input
        type="file"
        aria-label={t("Import YAML configuration")}
        accept=".yaml,.yml"
        onChange={async (e) => {
          const file = e.target.files?.[0];
          if (!file) return;
          if (file.size > 2 << 20) {
            setError(t("The configuration file must not exceed 2 MiB."));
            return;
          }
          setDraft(await file.text());
          onOpenYaml();
        }}
      />
      <h2>{t("Service lifecycle")}</h2>
      <p className="hint">
        {t(
          "Closing the browser keeps the service running. You can view historical logs after stopping the proxy. Quitting TokenHub takes the local management interface offline.",
        )}
      </p>
    </section>
  );
}
