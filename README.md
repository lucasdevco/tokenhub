# TokenHub

TokenHub 是本地 API 代理，用于转发请求并查看完整的请求与响应。支持 HTTP、SSE 流式响应和 WebSocket，适合调试 AI 客户端与模型服务。

- 将多个服务统一接入本地地址，如 OpenRouter、Anthropic、LM Studio。
- 在浏览器中管理服务商和账号，直接填写 API Key。
- 实时查看请求日志，搜索历史记录，下载原始数据。

## 开始使用

### 下载发布版本

在 [GitHub Releases](https://github.com/lucasdevco/tokenhub/releases) 下载 macOS 安装包或命令行版本：

- Apple Silicon 芯片选择 `darwin-arm64`，Intel 芯片选择 `darwin-amd64`。
- 桌面版下载 `tokenhub-darwin-<架构>.dmg`，打开后将 `tokenhub.app` 拖入「应用程序」。
- 命令行版下载 `tokenhub-cli-darwin-<架构>`，管理页面已嵌入程序，无需另外安装前端。
- `SHA256SUMS` 提供下载文件的 SHA-256 校验值。版本号保存在 Release 标题和 tag 中，附件名称保持固定。

以 Apple Silicon 的 CLI 为例：

```bash
chmod +x tokenhub-cli-darwin-arm64
./tokenhub-cli-darwin-arm64 -config "$HOME/.tokenhub/config.yaml"
```

首次启动会创建配置文件，随后在设置页添加服务商。Intel 芯片将命令中的 `arm64` 替换为 `amd64`。

`v0.1.0` 保留原命名：`tokenhub-v0.1.0-macos-<架构>.dmg`、`-cli.tar.gz` 和 `SHA256SUMS.txt`。旧 CLI 压缩包解压后运行 `./tokenhub -config config.example.yaml`。

当前安装包使用 ad hoc 签名，未经过 Apple 公证。macOS 可能阻止首次打开，确认下载来源后可在「系统设置 → 隐私与安全性」中允许打开。

### macOS 客户端

打开 `tokenhub.app`，点击菜单栏图标：

1. 打开「设置 → 服务商」，选择服务商或添加自定义服务，填写服务地址。
2. 添加账号，填写账号名称和 API Key；同一服务商可以保存多个账号。本地服务可不填密钥。
3. 保存服务商；如果代理尚未运行，在常用配置中点击「启动」。
4. 在「当前使用」中选择服务商和账号，切换立即保存并用于新请求，无需重启。每个服务商记住上次使用的账号。客户端始终使用固定代理地址和原生协议路径（如 `/v1`）。
5. 打开「请求日志」查看流量。

默认代理地址为 `http://127.0.0.1:5601`，设置页为 `http://127.0.0.1:5602/settings`，日志页为 `http://127.0.0.1:5602/logs/`。

应用数据目录默认为 `~/.tokenhub`：配置保存在 `config.yaml`，数据库为 `tokenhub.db`，程序日志为 `app.log`，请求日志默认保存在 `logs/`。首次启动没有 Provider，需要自行添加。停止代理后仍可查看历史日志；退出应用会停止服务。

### 从源码安装

安装 [mise](https://mise.jdx.dev/) 后，在项目目录执行：

```bash
mise trust
mise install
mise run setup

# 同时构建命令行程序和桌面应用
mise run build
```

命令行程序输出到 `build/tokenhub`；macOS App 和 DMG 输出到 `build/darwin-arm64/` 或 `build/darwin-amd64/`。仅构建桌面应用可运行 `mise run desktop`。构建不会自动替换「应用程序」中的已安装版本。

使用命令行版本（macOS / Linux）：

```bash
mise run build
cp config.example.yaml config.yaml
# 在 config.yaml 的账号中填写 API Key，或启动后在设置页添加账号
./build/tokenhub -config config.yaml
```

默认同时启动代理和管理页面。打开 `http://127.0.0.1:5602/settings` 管理配置，并启动、停止或重启代理；请求日志位于 `http://127.0.0.1:5602/logs/`。停止代理后，管理页面仍可使用。按 `Ctrl+C` 退出整个程序。代理启动失败时，管理页面仍保持运行，可修正配置后重新启动。

支持 `./build/tokenhub config.yaml` 指定配置文件，也可通过 `-listen 127.0.0.1:5603` 更改管理监听地址；管理地址不读取配置中的 `webui` 字段。CLI 和桌面应用默认使用相同端口，请分别运行。

原 `tokenhub-manager` 命令已合并到 `tokenhub`，无需再传 `-start`。

### 构建与发布

GitHub Actions 在推送 `main`、提交 PR 或手动运行时，检查前端格式、运行前端与两个 Go 模块的测试，并构建 Apple Silicon 和 Intel 的 DMG、独立 CLI 程序。构建结果保留在对应 Actions 运行的 Artifacts 中，保留 14 天。

发布新版本时，在通过构建的提交上创建并推送版本 tag：

```bash
git tag -a v0.1.1 -m "Release v0.1.1"
git push origin v0.1.1
```

将 `v0.1.1` 替换为新版本号。Actions 会将版本写入应用，验证 DMG 和 CLI，并在两个架构都成功后自动发布 GitHub Release，附带安装包、CLI 和校验文件。更新说明按功能、修复、改进等类别生成，并包含下载表格。`v0.1.1-rc.1` 这样的 tag 会发布为预发布版本。

也可在 Actions 的「Build and release → Run workflow」中填写已有 tag 的版本号，例如 `0.1.1`，勾选 `publish` 后发布。不勾选时只构建和上传 Artifacts；版本号留空时构建所选 ref。手动重建使用所选 tag 的源代码和当前工作流的打包规则。发布失败可重跑，同名附件和更新说明会被更新。

## 接入客户端

在「服务商」中添加 OpenRouter，填写 API Key 并选择账号，即可使用。命令行和高级配置也可使用以下写法：

```yaml
active_provider: openrouter
providers:
  openrouter:
    base_url: "https://openrouter.ai/api"
    active_account: personal
    accounts:
      - id: personal
        name: 个人账号
        key: "在这里填写你的 API Key"
```

Key 直接保存在账号配置中；不读取 `.env` 或进程环境中的密钥，不支持 `${变量名}` 引用。

将支持 OpenAI 兼容接口的客户端配置为：

```dotenv
OPENAI_BASE_URL=http://127.0.0.1:5601/v1
```

TokenHub 会将 `/v1/chat/completions` 转发到 `https://openrouter.ai/api/v1/chat/completions`，并自动添加目标服务的密钥。客户端如果要求填写 API Key，可填任意非空值；如果 Provider 设置了 `client_key`，则必须填写对应密钥。

其他常用 Providers：

| 服务 | Provider `base_url` | 客户端请求路径示例 |
| --- | --- | --- |
| 模力方舟 | `https://api.moark.com` | `/v1/chat/completions` |
| Anthropic | `https://api.anthropic.com` | `/v1/messages` |
| LM Studio | `http://127.0.0.1:1234` | `/v1/chat/completions` |
| llama.cpp | `http://127.0.0.1:8080` | `/v1/chat/completions` |

在「服务商」页的「当前使用」中切换上游或账号，会立即持久化并应用到新请求，不重启代理；已开始的 HTTP、SSE 和 WebSocket 请求继续完成。未选择 Provider 时返回 JSON 503 提示，上游的状态码（包括 404）和响应体保持透传。删除当前服务商会取消选择，需重新选择一个服务商。TokenHub 不转换协议，所选 Provider 必须支持客户端使用的接口。

Provider 的 `base_url` 是上游根地址，可包含固定基础路径（如 OpenRouter 的 `/api`），请求的完整路径和查询参数会追加到其后。客户端传 `/v1/...` 时，`base_url` 无需重复填写 `/v1`。

旧 `routes`、`pattern`、`destination` 和 `server.not_found` 已移除。升级时手动将各路由改为 `providers.<名称>.base_url`，移除客户端 URL 中的 Provider 前缀，并设置 `active_provider`；旧配置会明确报错，不自动迁移。

目标服务的认证方式不同，请按需设置认证请求头和密钥。未配置密钥替换时，客户端的认证头会原样转发。

## 常用设置

- **配置生效**：「服务商」页保存地址、账号和鉴权设置后立即用于新请求；修改常用配置或原始配置仍需重启代理。
- **Provider 切换**：未保存的草稿需先保存或放弃；若已保存的配置待应用，先重启再切换，避免隐式应用其他修改。
- **账号密钥**：直接在账号表单中填写，保存到 `providers.<id>.accounts[].key`。已移除环境文件读取、变量引用和环境切换，不提供旧环境配置迁移。
- **上游代理**：在配置中设置 `http_client.proxy_url`，支持 HTTP 和 SOCKS5，例如 `http://127.0.0.1:6152`。留空时默认读取进程代理环境变量；示例配置已关闭此行为。每个服务商可在「高级设置」中单独关闭「启用上游代理」（`providers.<id>.proxy_enabled: false`），直接连接上游；默认开启并遵循全局代理设置，保存后对新请求立即生效。
- **日志目录**：由 `logging.log_dir` 指定，相对路径以配置文件所在目录为基准。可为单个 Provider 设置 `logging: false` 关闭记录。
- **程序日志**：菜单栏点击「程序日志」打开 `app.log`，记录程序启动、服务监听、停止与运行错误。文件位于配置文件旁，macOS 默认路径为 `~/.tokenhub/app.log`；命令行版本也会保存到配置文件旁，并继续输出到终端。程序日志不受请求日志开关影响，重启后追加保留。

日志包含请求与响应的头部、正文和流式数据，可能包含密钥及私人内容，分享前请检查。

## 代理与日志架构

- `internal/proxy` 负责 Provider 选择和认证，HTTP、SSE、WebSocket 统一通过 Go 标准库 `httputil.ReverseProxy` 转发，共享上游连接池。重定向由客户端决定，状态码、压缩内容和 Trailer 保持透传。
- `internal/capture` 按交易管理请求和响应采集。每个方向最多缓冲 256 KiB，所有 Recorder 共享 32 MiB 队列预算；预算仅限制排队数据，不包含协议解析器和 HTTP 连接内存。缓冲满时等待写入，优先保留完整日志。取消或停机仍会保存已入队的数据，并记录未完成原因。
- 每笔交易保存请求、响应两个 `.bin` 文件和一个小型 `_metadata.jsonl` 文件。正文不再复制到 metadata；HTTP 捕获文件保存重建的头部和解压正文，未知压缩格式保留原始头部和字节；WebSocket 保存握手和帧。新格式不提供历史日志兼容或迁移。
- 请求结果与采集结果分别记录。磁盘写入失败不会改写成功的 HTTP 响应，日志页会显示采集失败；整个日志目录不可写时，失败原因写入程序日志。
- 首 Token 耗时使用数据接收时间，排队或写盘时间不会冒充接收时间；压缩流无法可靠定位事件时间时，不展示 TTFT 和生成速度。详情提供两个方向累计的采集等待时间。
- `internal/database` 使用纯 Go 驱动 [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) 保存持久化索引和待处理通知，SQL 查询由 [sqlc](https://github.com/sqlc-dev/sqlc) 生成。构建无需 CGO 或 C 编译器，运行时无需安装 sqlc 或 SQLite 服务；已有 SQLite 数据库可继续使用。
- 数据库统一保存在配置文件所在目录的 `tokenhub.db`，不跟随 `logging.log_dir`。macOS 默认位置为 `~/.tokenhub/tokenhub.db`。日志索引以来源目录隔离，切换日志目录不会混用记录、统计或待处理通知。
- `internal/management` 统一提供设置、运行状态和请求日志接口；启动只打开当前数据库，不扫描或自动导入历史日志，也不读取、迁移或删除旧的 `.tokenhub-index.sqlite`。
- 新日志写入前后更新 SQLite 待处理记录，页面每 250 ms 分批处理，关闭页面期间的更新也能保留。原内存 `Changes` 队列已移除。活跃记录按固定批次刷新；没有定时全目录扫描。记录正文仍保存在 `.bin` 文件中，按需读取。
- 列表使用时间与 ID 游标翻页，总数和趋势图使用预聚合表。任意子串搜索仍可能扫描大量索引数据，查询有 2 秒超时；这不是亿级全历史搜索延迟的保证。SQLite 索引文件及其 WAL/SHM 文件不会通过日志下载接口暴露。

SQL 源文件位于 `internal/database/sql/`，修改后运行 `mise run generate:sql`，提交生成的 `internal/database/queries/`，不要手工修改生成代码。普通构建直接使用已生成代码。

外部新增、修改或删除历史文件后，可显式重建／校验索引；不会启动代理，也不会修改原始日志：

```bash
./build/tokenhub -config config.yaml -reindex-logs
```

重建只在显式执行上述命令时运行，不在正常启动时自动执行。`tokenhub.db` 是应用级数据库；备份时一同保存配置目录和日志目录。旧数据库不提供兼容或迁移。

退出时先停止代理连接，再等待采集与最终元数据写入。超时会标记未完成；这里的完整性指正常运行和正常退出时不主动丢弃数据，不承诺断电时已经落入操作系统缓存的数据持久化。
