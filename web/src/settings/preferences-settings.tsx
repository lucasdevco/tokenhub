import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { request as api } from "./settings-api";
import { Field, Toggle } from "./settings-controls";

export type Preferences = { auto_start: boolean; open_at_login: boolean };
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
  return (
    <section>
      <h1>客户端偏好</h1>
      <p className="intro">偏好单独保存，不需要重启代理。</p>
      {prefs && (
        <>
          <Field label="自动启动服务">
            <Toggle
              label="打开客户端时启动代理"
              disabled={locked}
              value={prefs.auto_start}
              onChange={(value) =>
                perform(async () => {
                  const next = { ...prefs, auto_start: value };
                  await api("/api/preferences", next);
                  setPrefs(next);
                  setNotice("偏好已保存。");
                })
              }
            />
          </Field>
          <Field
            label="登录时启动"
            help={
              loginSupported
                ? "需要安装打包后的 macOS 应用。"
                : "此选项需在 macOS 菜单栏客户端中使用。"
            }
          >
            <Toggle
              label="登录 macOS 时打开 TokenHub"
              disabled={locked || !loginSupported}
              value={prefs.open_at_login}
              onChange={(value) =>
                perform(async () => {
                  const next = { ...prefs, open_at_login: value };
                  await api("/api/preferences", next);
                  setPrefs(next);
                  setNotice("登录启动设置已更新。");
                })
              }
            />
          </Field>
        </>
      )}
      <h2>配置文件</h2>
      <code className="config-path">{filename}</code>
      <Button
        variant="outline"
        size="sm"
        className="open-config-directory"
        disabled={locked || !finderSupported}
        title={
          finderSupported ? "打开配置文件所在目录" : "需要 macOS 桌面客户端"
        }
        onClick={() =>
          perform(async () => {
            await api("/api/directories/config/open", {});
          })
        }
      >
        Open in Finder
      </Button>
      <h2>导入已有配置</h2>
      <p className="hint">
        导入只替换当前草稿，保存前会校验。相对路径按当前配置目录解析。
      </p>
      <Input
        type="file"
        aria-label="导入 YAML 配置"
        accept=".yaml,.yml"
        onChange={async (e) => {
          const file = e.target.files?.[0];
          if (!file) return;
          if (file.size > 2 << 20) {
            setError("配置文件不能超过 2 MiB。");
            return;
          }
          setDraft(await file.text());
          onOpenYaml();
        }}
      />
      <h2>服务生命周期</h2>
      <p className="hint">
        关闭浏览器不停止服务。停止代理后仍可查看历史日志；退出 TokenHub
        后，本机管理网页离线。
      </p>
    </section>
  );
}
