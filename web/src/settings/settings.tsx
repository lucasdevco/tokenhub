import { ServiceControls } from "../service-controls";
import { Button } from "../components/ui/button";
import { Alert, AlertDescription } from "../components/ui/alert";
import {
  Link,
  Outlet,
  useBlocker,
  useLocation,
  useNavigate,
} from "@tanstack/react-router";
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { parseDocument } from "yaml";
import PreferencesSettings, { type Preferences } from "./preferences-settings";
import YamlSettings from "./yaml-settings";
import ProviderSettings from "./provider-settings";
import BasicSettings from "./basic-settings";
import { readDraft } from "./config";
import { request as api, setSession } from "./settings-api";
import Sidebar, { type SettingsTab, settingsTab } from "../sidebar";

type Status = {
  state: string;
  error?: string;
  pending: boolean;
  address: string;
  config_file: string;
  active_provider?: string;
};

type ConfigResponse = { yaml: string; version: string; filename: string };
type Tab = SettingsTab;
const SettingsPages = createContext<Partial<Record<Tab, ReactNode>>>({});

export function SettingsPage({ tab }: { tab: Tab }) {
  return useContext(SettingsPages)[tab];
}

const labels: Record<string, string> = {
  stopped: "已停止",
  starting: "启动中",
  running: "运行中",
  stopping: "停止中",
  error: "启动失败",
};

export default function Settings() {
  const tab = useLocation({
    select: (location) => settingsTab(location.pathname),
  });
  const navigate = useNavigate();
  const [config, setConfig] = useState<ConfigResponse>();
  const [draft, setDraft] = useState("");
  const [status, setStatus] = useState<Status>();
  const [prefs, setPrefs] = useState<Preferences>();
  const [loginSupported, setLoginSupported] = useState(false);
  const [finderSupported, setFinderSupported] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [offline, setOffline] = useState(false);
  const [copiedAddress, setCopiedAddress] = useState("");
  useEffect(() => {
    if (!copiedAddress) return;
    const timer = setTimeout(() => setCopiedAddress(""), 2000);
    return () => clearTimeout(timer);
  }, [copiedAddress]);
  const { config: cfg, error: yamlError } = readDraft(draft);
  const dirty = !!config && draft !== config.yaml;
  useEffect(() => {
    document.title = "设置 · TokenHub";
    let cancelled = false;
    async function load() {
      try {
        setSession((await api<{ token: string }>("/api/session")).token);
        const [c, s, p, capabilities] = await Promise.all([
          api<ConfigResponse>("/api/config"),
          api<Status>("/api/status"),
          api<{ preferences: Preferences; login_supported: boolean }>(
            "/api/preferences",
          ),
          api<{ finder: boolean }>("/api/capabilities"),
        ]);
        if (cancelled) return;
        setConfig(c);
        setDraft(c.yaml);
        setStatus(s);
        setPrefs(p.preferences);
        setLoginSupported(p.login_supported);
        setFinderSupported(capabilities.finder);
        try {
          if (readDraft(c.yaml).error) choose("yaml", true);
        } catch {
          choose("yaml", true);
        }
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    }
    void load();
    const timer = setInterval(() => {
      api<Status>("/api/status")
        .then((s) => {
          if (!cancelled) {
            setStatus(s);
            setOffline(false);
          }
        })
        .catch(() => {
          if (!cancelled) setOffline(true);
        });
    }, 1500);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, []);
  useBlocker({
    enableBeforeUnload: dirty,
    shouldBlockFn: ({ next }) => {
      if (
        dirty &&
        (!next.pathname.startsWith("/settings/") ||
          next.pathname === "/settings/providers")
      ) {
        return !confirm("离开会放弃未保存的配置修改，继续？");
      }
      return false;
    },
  });
  function edit(path: (string | number)[], value: unknown) {
    const next = parseDocument(draft);
    if (next.errors.length) {
      setError("YAML 存在语法错误，请先到完整 YAML 页面修复。");
      return;
    }
    if (value === undefined) next.deleteIn(path);
    else next.setIn(path, value);
    setDraft(next.toString());
    setNotice("");
  }
  function choose(next: Tab, replace = false) {
    void navigate({ to: `/settings/${next}`, replace });
  }
  async function perform(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  async function reload() {
    const next = await api<ConfigResponse>("/api/config");
    setConfig(next);
    setDraft(next.yaml);
    try {
      if (readDraft(next.yaml).error) choose("yaml", true);
    } catch {
      choose("yaml", true);
    }
  }
  async function restart() {
    setRestarting(true);
    const started = Date.now();
    let next: Status;
    try {
      next = await api<Status>("/api/service/restart", {});
    } finally {
      // Keep feedback visible even when the local restart finishes immediately.
      await new Promise((resolve) =>
        setTimeout(resolve, Math.max(0, 600 - (Date.now() - started))),
      );
      setRestarting(false);
    }
    setStatus(next);
    setNotice("服务已重启，已应用当前保存的配置。");
  }
  async function save() {
    if (!config) return;
    await api("/api/config/validate", { yaml: draft });
    const result = await api<{ version: string }>("/api/config", {
      yaml: draft,
      version: config.version,
    });
    setConfig({ ...config, yaml: draft, version: result.version });
    setStatus(await api("/api/status"));
    setNotice(
      "配置已保存。" +
        (status?.state === "running"
          ? "重启代理后生效。"
          : "下次启动代理时生效。"),
    );
  }
  const locked =
    busy || offline || ["starting", "stopping"].includes(status?.state || "");
  const providerHelp = dirty
    ? "请先保存或放弃当前修改，再切换 Provider。"
    : status?.pending
      ? "配置待应用，请先重启代理再切换。"
      : "切换立即保存并生效，新请求使用所选 Provider；正在进行的请求不受影响。";
  return (
    <div className="app-shell">
      <header className="topbar">
        <Link className="brand" to="/">
          TokenHub
        </Link>
        <span className={`status ${status?.state || "stopped"}`}>
          <i />
          {offline
            ? "管理服务离线"
            : restarting
              ? "重启中…"
              : labels[status?.state || "stopped"]}
        </span>
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
          providers={Object.keys(cfg?.providers || {})}
          providerLabels={Object.fromEntries(
            Object.entries(cfg?.providers || {}).map(([id, provider]) => [
              id,
              provider.name || id,
            ]),
          )}
          activeProvider={status?.active_provider ?? cfg?.active_provider ?? ""}
          locked={locked}
          switchingLocked={dirty}
          providerLocked={!config || !cfg || !status || status.pending}
          providerHelp={providerHelp}
          canRestart={status?.state === "running"}
          pending={status?.pending}
          restarting={restarting}
          onRestart={() => void perform(restart)}
          onAccountChange={(account) => {
            if (!config || !status?.active_provider) return;
            void perform(async () => {
              const result = await api<{ status: Status }>("/api/providers", {
                action: "account",
                provider: status.active_provider,
                account,
                version: config.version,
              });
              const next = await api<ConfigResponse>("/api/config");
              setConfig(next);
              setDraft(next.yaml);
              setStatus(result.status);
              setNotice("账号已切换，新请求立即生效。");
            });
          }}
          onProviderChange={(provider) => {
            if (!config) return;
            void perform(async () => {
              const updated = await api<ConfigResponse>("/api/provider", {
                provider,
                version: config.version,
              });
              setConfig(updated);
              setDraft(updated.yaml);
              setStatus(await api<Status>("/api/status"));
              setNotice(
                provider
                  ? `已切换到 ${provider}。`
                  : "已停用 Provider，新请求将返回 503。",
              );
            });
          }}
        />
      </header>
      <div className="layout">
        <Sidebar active={tab} />
        <main>
          {tab === "basic" && (
            <div className="service-bar">
              <div>
                {status?.state === "running" && status.address ? (
                  <div className="endpoint">
                    <span className="endpoint-label">代理地址</span>
                    <div className="endpoint-row">
                      <code title={status.address}>{status.address}</code>
                      <Button
                        variant="outline"
                        size="sm"
                        className="copy-address"
                        disabled={locked}
                        onClick={async () => {
                          try {
                            await navigator.clipboard.writeText(status.address);
                            setCopiedAddress(status.address);
                          } catch {
                            setError("复制失败，请选中地址后手动复制。");
                          }
                        }}
                      >
                        <span aria-live="polite">
                          {copiedAddress === status.address
                            ? "已复制"
                            : "复制地址"}
                        </span>
                      </Button>
                    </div>
                  </div>
                ) : (
                  <strong>代理尚未启动</strong>
                )}
                {status?.pending && <p>配置已保存，重启后生效。</p>}
              </div>
              <div className="buttons">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={locked || status?.state === "running"}
                  onClick={() =>
                    perform(async () => {
                      setStatus(await api("/api/service/start", {}));
                    })
                  }
                >
                  启动
                </Button>

                <Button
                  variant="outline"
                  size="sm"
                  disabled={locked || status?.state !== "running"}
                  onClick={() =>
                    perform(async () => {
                      setStatus(await api("/api/service/stop", {}));
                    })
                  }
                >
                  停止
                </Button>
              </div>
            </div>
          )}
          {offline && (
            <Alert variant="destructive" className="message error" role="alert">
              <AlertDescription>
                无法连接 TokenHub。请从菜单栏重新打开客户端，再刷新页面。
              </AlertDescription>
            </Alert>
          )}
          {(error || status?.error) && (
            <Alert variant="destructive" className="message error" role="alert">
              <AlertDescription>{error || status?.error}</AlertDescription>
            </Alert>
          )}
          {notice && (
            <Alert variant="default" className="message success" role="status">
              <AlertDescription>{notice}</AlertDescription>
            </Alert>
          )}
          {!config ? (
            <div className="empty">
              {error ? "配置加载失败。" : "正在加载配置…"}
              {error && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => location.reload()}
                >
                  重新加载
                </Button>
              )}
            </div>
          ) : (
            <>
              {tab !== "yaml" && !cfg && (
                <div className="message error">
                  YAML 无法解析。
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => choose("yaml")}
                  >
                    打开完整 YAML
                  </Button>
                </div>
              )}
              <SettingsPages.Provider
                value={{
                  basic: cfg && (
                    <BasicSettings
                      config={cfg}
                      edit={edit}
                      locked={locked}
                      finderSupported={finderSupported}
                      perform={perform}
                    />
                  ),
                  providers: <ProviderSettings />,
                  yaml: (
                    <YamlSettings
                      draft={draft}
                      setDraft={setDraft}
                      yamlError={yamlError}
                      locked={locked}
                      setNotice={setNotice}
                      perform={perform}
                    />
                  ),
                  app: (
                    <PreferencesSettings
                      prefs={prefs}
                      setPrefs={setPrefs}
                      filename={config.filename}
                      locked={locked}
                      loginSupported={loginSupported}
                      finderSupported={finderSupported}
                      setError={setError}
                      setNotice={setNotice}
                      setDraft={setDraft}
                      onOpenYaml={() => choose("yaml")}
                      perform={perform}
                    />
                  ),
                }}
              >
                <Outlet />
              </SettingsPages.Provider>
              {
                <footer className="savebar">
                  <span>
                    {dirty
                      ? "有未保存的修改"
                      : status?.pending
                        ? "已保存，待重启"
                        : "配置已保存"}
                  </span>
                  <div className="buttons">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={locked}
                      onClick={() => {
                        if (dirty && !confirm("重新加载会放弃当前草稿，继续？"))
                          return;
                        void perform(reload);
                      }}
                    >
                      重新加载
                    </Button>
                    <Button
                      size="sm"
                      className="primary"
                      disabled={locked || !dirty || !!yamlError}
                      onClick={() => perform(save)}
                    >
                      保存配置
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={
                        locked ||
                        dirty ||
                        !status?.pending ||
                        status?.state !== "running"
                      }
                      onClick={() => perform(restart)}
                    >
                      {restarting ? "重启中…" : "重启并应用"}
                    </Button>
                  </div>
                </footer>
              }
            </>
          )}
        </main>
      </div>
    </div>
  );
}
