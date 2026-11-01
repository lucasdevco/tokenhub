# TokenHub

[English](README.md) | 简体中文

**本地 AI API 可观测工具，看清请求、响应、Token 用量和耗时。**

TokenHub 是面向 macOS 的本地 AI API 可观测工具。通过本机代理记录客户端的 API 流量，在浏览器中查看提示词、工具调用、流式响应、Token 用量和耗时，排查请求失败、回答异常和响应缓慢的问题。同时支持从菜单栏切换服务商和账号。

## 为什么使用 TokenHub

- **看清 AI 请求与响应。** 查看提示词、模型参数、工具调用和流式内容，了解客户端发送了什么、模型返回了什么。
- **排查失败与慢请求。** 查看请求状态和耗时，包括首 Token 耗时和生成速度。
- **查看 Token 用量和历史记录。** 查看每次请求的 Token 用量，搜索历史请求，下载原始数据进行分析。日志保存在本机，停止代理后仍可查看。
- **切换服务，不用反复改客户端。** 统一使用本地地址，在 TokenHub 中切换 OpenRouter、Anthropic、LM Studio 等服务，以及同一服务下的多个账号。

TokenHub 保留客户端原本的 API 协议，不做协议转换。切换服务时，目标服务需要支持客户端使用的接口和模型。

## 开始使用

### 1. 安装并打开

从 [GitHub Releases](https://github.com/lucasdevco/tokenhub/releases) 下载 macOS 安装包：

| Mac 芯片 | 选择的文件 |
| --- | --- |
| Apple Silicon（M 系列） | `tokenhub-darwin-arm64.dmg` |
| Intel | `tokenhub-darwin-amd64.dmg` |

打开 DMG，将 `tokenhub.app` 拖入「应用程序」后打开，菜单栏会出现 TokenHub 图标。

### 2. 添加服务商和账号

1. 点击菜单栏图标，打开「设置 → 服务商」。
2. 选择服务商或添加自定义服务，填写服务地址。例如，OpenRouter 填写 `https://openrouter.ai/api`。
3. 添加账号，填写名称和 API Key，然后保存。本地模型服务可不填密钥。
4. 在「当前使用」中选择服务商和账号。如果代理尚未运行，点击「启动服务」。

以后可以直接从菜单栏切换服务商和账号，新请求立即使用所选账号，无需重启。

### 3. 接入你的 AI 客户端

以支持 OpenAI 兼容接口的客户端为例，在客户端设置中填写：

| 设置 | 填写内容 |
| --- | --- |
| Base URL / API 地址 | `http://127.0.0.1:5601/v1` |
| API Key | 任意非空值，例如 `tokenhub` |
| 模型 | 所选服务商支持的模型名称 |

真实的 API Key 由 TokenHub 从所选账号中添加。如果你设置了本地客户端密钥（`client_key`），客户端的 API Key 需填写该密钥。

使用 Anthropic 接口时，将客户端的 API 根地址设为 `http://127.0.0.1:5601`，并在 TokenHub 中选择支持 Anthropic 协议的服务。

### 4. 查看请求日志

在客户端发起一次对话，然后点击菜单栏的「请求日志」，或打开 [日志页面](http://127.0.0.1:5602/logs/)。点击一条记录即可查看请求、响应、耗时和 Token 用量。

设置页面位于 [http://127.0.0.1:5602/settings](http://127.0.0.1:5602/settings)。

## 命令行运行

不使用菜单栏应用时，也可以从 Releases 下载对应芯片的 CLI。以 Apple Silicon 为例：

```bash
chmod +x tokenhub-cli-darwin-arm64
./tokenhub-cli-darwin-arm64 -config "$HOME/.tokenhub/config.yaml"
```

首次启动会创建配置文件。打开设置页面，按上面的步骤添加服务商和账号即可。按 `Ctrl+C` 退出。桌面版与命令行版使用相同端口，请分别运行。

## 数据与隐私

配置和日志保存在本机，桌面版默认目录为 `~/.tokenhub`。API Key 明文保存在配置文件中；请求日志可能包含密钥和私人对话，分享前请检查。停止代理后仍可查看历史日志，退出 TokenHub 会停止代理服务。
