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
  return (
    <section>
      <h1>常用配置</h1>
      <p className="intro">
        配置代理监听、流量记录和出站连接。保存不会中断正在运行的代理。
      </p>
      <h2>反向代理</h2>
      <Field label="启用监听">
        <Toggle
          label="启用反向代理"
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
          <Field label="监听地址">
            <Input
              value={config.server.host || ""}
              onChange={(e) => edit(["server", "host"], e.target.value)}
              placeholder="127.0.0.1"
            />
          </Field>
          <Field label="监听端口">
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
      <h2>请求记录</h2>
      <Field label="记录请求和响应">
        <Toggle
          label="启用磁盘日志"
          value={!!config.logging?.enabled}
          onChange={(value) => edit(["logging", "enabled"], value)}
        />
      </Field>
      <Field
        inputId="logs-directory"
        label="日志目录"
        help="相对路径以配置文件所在目录为基准。"
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
                ? "打开当前使用的日志目录"
                : "需要 macOS 桌面客户端"
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
      <Field label="控制台输出">
        <Toggle
          label="输出到服务控制台"
          value={!!config.logging?.console}
          onChange={(value) => edit(["logging", "console"], value)}
        />
      </Field>
      <h2>出站连接</h2>
      <Field label="上游代理" help="留空时按下方选项决定是否使用代理环境变量。">
        <Input
          value={config.http_client?.proxy_url || ""}
          onChange={(e) => edit(["http_client", "proxy_url"], e.target.value)}
          placeholder="http://127.0.0.1:6152"
        />
      </Field>
      <Field label="环境变量">
        <Toggle
          label="继承 HTTP_PROXY / HTTPS_PROXY 等变量"
          value={config.http_client?.proxy_from_environment !== false}
          onChange={(value) =>
            edit(["http_client", "proxy_from_environment"], value)
          }
        />
      </Field>
      <h2>管理网页</h2>
      <Field label="当前地址">
        <code>{location.origin}</code>
      </Field>
      <p className="hint">
        管理网页独立于代理运行。管理监听端口通过启动参数配置，不随代理重启而改变。
      </p>
    </section>
  );
}
