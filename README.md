# TokenHub

English | [简体中文](README.zh-CN.md)

**Local observability for AI APIs. Inspect requests, responses, token usage, and latency.**

TokenHub is a local AI API observability tool for macOS. Its proxy captures your client's API traffic so you can inspect prompts, tool calls, streamed responses, token usage, and timing in your browser. Use it to investigate failed requests, unexpected answers, and slow responses. You can also switch providers and accounts from the menu bar.

## Why use TokenHub?

- **Inspect AI requests and responses.** See prompts, model parameters, tool calls, and streamed content to understand what your client sends and what the model returns.
- **Investigate failures and slow responses.** Inspect request status and timing, including time to first token and generation speed.
- **Track token usage and review history.** View token usage per request, search past requests, and download raw data for further analysis. Logs stay on your machine and remain available while the proxy is stopped.
- **Switch providers without reconfiguring your client.** Use one local address for OpenRouter, Anthropic, LM Studio, and other services, with multiple accounts per provider.

TokenHub preserves your client's API protocol. It does not convert between protocols, so the selected provider must support the interface and model your client uses.

## Get started

### 1. Install and open

Download the macOS installer from [GitHub Releases](https://github.com/lucasdevco/tokenhub/releases):

| Mac chip | Download |
| --- | --- |
| Apple Silicon (M series) | `tokenhub-darwin-arm64.dmg` |
| Intel | `tokenhub-darwin-amd64.dmg` |

Open the DMG, drag `tokenhub.app` into Applications, and open it. The TokenHub icon appears in your menu bar.

### 2. Add a provider and account

1. Click the menu bar icon and open Settings → Providers (「设置 → 服务商」).
2. Select a provider or add a custom service and enter its address. For OpenRouter, use `https://openrouter.ai/api`.
3. Add an account, enter its name and API key, and save. Local model services can use an empty key.
4. Select the provider and account under Current selection (「当前使用」). If the proxy is stopped, click Start service (「启动服务」).

You can then switch providers and accounts from the menu bar. New requests use your selection immediately, with no restart.

### 3. Connect your AI client

For a client that supports the OpenAI-compatible API, enter these settings:

| Setting | Value |
| --- | --- |
| Base URL / API address | `http://127.0.0.1:5601/v1` |
| API key | Any non-empty value, such as `tokenhub` |
| Model | A model supported by the selected provider |

TokenHub adds the actual API key from the selected account. If you have configured a local client key (`client_key`), enter that key in your client instead.

For the Anthropic API, set your client's API base URL to `http://127.0.0.1:5601` and select a provider that supports the Anthropic protocol.

### 4. Inspect requests

Start a conversation in your client, then click Request logs (「请求日志」) in the menu bar or open the [logs page](http://127.0.0.1:5602/logs/). Click a record to inspect its request, response, timing, and token usage.

Settings are available at [http://127.0.0.1:5602/settings](http://127.0.0.1:5602/settings).

## Run from the command line

To run without the menu bar app, download the CLI for your Mac from Releases. On Apple Silicon:

```bash
chmod +x tokenhub-cli-darwin-arm64
./tokenhub-cli-darwin-arm64 -config "$HOME/.tokenhub/config.yaml"
```

The first run creates a configuration file. Open the settings page and follow the steps above to add a provider and account. Press `Ctrl+C` to quit. The desktop app and CLI use the same ports, so run one at a time.

## Data and privacy

Configuration and logs are stored locally. The desktop app uses `~/.tokenhub` by default. API keys are stored in plain text in the configuration file. Request logs may contain keys and private conversations, so review them before sharing. You can browse past logs while the proxy is stopped. Quitting TokenHub stops the proxy.
