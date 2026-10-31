import { t, useLocale } from "../i18n";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import type { Configuration } from "./config";
import { request as api } from "./settings-api";
import { Field, Toggle } from "./settings-controls";

type Props = {
  config: Configuration;
  edit: (path: (string | number)[], value: unknown) => void;
  locked: boolean;
  finderSupported: boolean;
  perform: (action: () => Promise<void>) => Promise<void>;
};

export default function BasicSettings({
  config,
  edit,
  locked,
  finderSupported,
  perform,
}: Props) {
  useLocale();
  return (
    <section>
      <h1>{t("General settings")}</h1>
      <p className="intro">
        {t(
          "Configure the proxy listener, traffic recording, and outbound connections. Saving does not interrupt the running proxy.",
        )}
      </p>
      <h2>{t("Reverse proxy")}</h2>
      <Field label={t("Listener")}>
        <Toggle
          label={t("Enable reverse proxy")}
          value={!!config.server}
          onChange={(value) =>
            edit(
              ["server"],
              value ? { host: "127.0.0.1", port: 5601 } : undefined,
            )
          }
        />
      </Field>
      {config.server && (
        <>
          <Field label={t("Listen address")}>
            <Input
              value={config.server.host || ""}
              onChange={(e) => edit(["server", "host"], e.target.value)}
              placeholder="127.0.0.1"
            />
          </Field>
          <Field label={t("Listen port")}>
            <Input
              type="number"
              min="1"
              max="65535"
              value={config.server.port ?? 5601}
              onChange={(e) => edit(["server", "port"], Number(e.target.value))}
            />
          </Field>
        </>
      )}
      <h2>{t("Request recording")}</h2>
      <Field label={t("Record requests and responses")}>
        <Toggle
          label={t("Enable disk logging")}
          value={!!config.logging?.enabled}
          onChange={(value) => edit(["logging", "enabled"], value)}
        />
      </Field>
      <Field
        inputId="logs-directory"
        label={t("Log directory")}
        help={t(
          "Relative paths are resolved from the configuration directory.",
        )}
      >
        <div className="directory-field">
          <Input
            id="logs-directory"
            aria-describedby="logs-directory-help"
            value={config.logging?.log_dir || ""}
            onChange={(e) => edit(["logging", "log_dir"], e.target.value)}
            placeholder="logs"
          />
          <Button
            variant="outline"
            size="sm"
            type="button"
            disabled={locked || !finderSupported}
            title={
              finderSupported
                ? t("Open the active log directory")
                : t("Requires the macOS desktop client")
            }
            onClick={() =>
              perform(async () => {
                await api("/api/directories/logs/open", {});
              })
            }
          >
            Open in Finder
          </Button>
        </div>
      </Field>
      <Field label={t("Console output")}>
        <Toggle
          label={t("Write to service console")}
          value={!!config.logging?.console}
          onChange={(value) => edit(["logging", "console"], value)}
        />
      </Field>
      <h2>{t("Outbound connections")}</h2>
      <Field
        label={t("Upstream proxy")}
        help={t(
          "Leave blank to use the proxy environment variables if enabled below.",
        )}
      >
        <Input
          value={config.http_client?.proxy_url || ""}
          onChange={(e) => edit(["http_client", "proxy_url"], e.target.value)}
          placeholder="http://127.0.0.1:6152"
        />
      </Field>
      <Field label={t("Environment variables")}>
        <Toggle
          label={t("Inherit HTTP_PROXY / HTTPS_PROXY and related variables")}
          value={config.http_client?.proxy_from_environment !== false}
          onChange={(value) =>
            edit(["http_client", "proxy_from_environment"], value)
          }
        />
      </Field>
      <h2>{t("Management interface")}</h2>
      <Field label={t("Current URL")}>
        <code>{location.origin}</code>
      </Field>
      <p className="hint">
        {t(
          "The management interface runs independently of the proxy. Its listen port is set by startup arguments and does not change when the proxy restarts.",
        )}
      </p>
    </section>
  );
}
