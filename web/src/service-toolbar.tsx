import { t, useLocale } from "./i18n";
import { useEffect, useState } from "react";
import { ServiceControls } from "./service-controls";
import { request, setSession } from "./settings/settings-api";
import { readDraft } from "./settings/config";

type Status = { state: string; pending: boolean; active_provider?: string };
type Config = { yaml: string; version: string };

// Read-only pages have no settings draft to protect.
export default function ServiceToolbar() {
  useLocale();
  const [available, setAvailable] = useState(false);
  const [config, setConfig] = useState<Config>();
  const [status, setStatus] = useState<Status>();
  const [busy, setBusy] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [offline, setOffline] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  useEffect(() => {
    let active = true;
    let supported = false;
    async function load() {
      try {
        const capabilities = await request<{ settings: boolean }>(
          "/api/capabilities",
        );
        if (!active || !capabilities.settings) return;
        supported = true;
        setAvailable(true);
        setSession((await request<{ token: string }>("/api/session")).token);
        const [config, status] = await Promise.all([
          request<Config>("/api/config"),
          request<Status>("/api/status"),
        ]);
        if (!active) return;
        setConfig(config);
        setStatus(status);
      } catch (error) {
        if (active) setError(String(error));
      }
    }
    void load();
    const timer = setInterval(() => {
      if (!supported) return;
      request<Status>("/api/status")
        .then((status) => {
          if (active) {
            setStatus(status);
            setOffline(false);
          }
        })
        .catch(() => {
          if (active) setOffline(true);
        });
    }, 1500);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, []);

  async function perform(action: () => Promise<string>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const message = await action();
      setStatus(await request<Status>("/api/status"));
      setNotice(message);
    } catch (error) {
      setError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
  if (!available) return null;
  const cfg = config && readDraft(config.yaml).config;
  const locked =
    busy ||
    offline ||
    !status ||
    ["starting", "stopping"].includes(status.state);
  return (
    <>
      <ServiceControls
        accounts={
          cfg?.providers?.[
            status?.active_provider ?? cfg?.active_provider ?? ""
          ]?.accounts || []
        }
        activeAccount={
          cfg?.providers?.[
            status?.active_provider ?? cfg?.active_provider ?? ""
          ]?.active_account || ""
        }
        providerLabels={Object.fromEntries(
          Object.entries(cfg?.providers || {}).map(([id, provider]) => [
            id,
            provider.name || id,
          ]),
        )}
        providers={Object.keys(cfg?.providers || {})}
        activeProvider={status?.active_provider ?? cfg?.active_provider ?? ""}
        locked={locked}
        providerLocked={!config || !cfg || status?.pending}
        providerHelp={
          status?.pending
            ? t("Configuration pending. Restart the proxy before switching.")
            : t(
                "The selection is saved immediately. New requests use the selected provider.",
              )
        }
        canRestart={status?.state === "running"}
        pending={status?.pending}
        restarting={restarting}
        onAccountChange={(account) => {
          if (!config || !status?.active_provider) return;
          void perform(async () => {
            await request("/api/providers", {
              action: "account",
              provider: status.active_provider,
              account,
              version: config.version,
            });
            setConfig(await request<Config>("/api/config"));
            return t("Account switched. New requests use it immediately.");
          });
        }}
        onProviderChange={(provider) => {
          if (!config) return;
          void perform(async () => {
            setConfig(
              await request<Config>("/api/provider", {
                provider,
                version: config.version,
              }),
            );
            return provider
              ? t("Switched to {0}.", [provider])
              : t("Provider disabled. New requests will return 503.");
          });
        }}
        onRestart={() =>
          void perform(async () => {
            setRestarting(true);
            const started = Date.now();
            try {
              setStatus(await request<Status>("/api/service/restart", {}));
            } finally {
              await new Promise((resolve) =>
                setTimeout(resolve, Math.max(0, 600 - (Date.now() - started))),
              );
              setRestarting(false);
            }
            return t("Service restarted with the saved configuration.");
          })
        }
      />
      {(error || notice || offline) && (
        <div
          className="topbar-feedback"
          role={error || offline ? "alert" : "status"}
        >
          {error || (offline ? t("Management service offline") : notice)}
        </div>
      )}
    </>
  );
}
