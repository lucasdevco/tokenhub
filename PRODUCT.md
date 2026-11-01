# tokenhub

<!-- impeccable:product-schema 1 -->

## Platform

adaptive

## Stack

MyGo native macOS menu-bar panel. Browser settings and request logs use
React, TypeScript, Vite, Tailwind CSS and pnpm; Go embeds the build output.
Reference: lucasdevco/gk-template. Existing Go proxy and read-only log viewer
remain shared capabilities.

## Product Purpose

Make AI API traffic observable locally. Capture requests and responses through a
local proxy, then inspect prompts, tool calls, streamed content, token usage and
timing in the system browser to investigate failures and slow responses.
The macOS menu bar provides proxy controls and provider and account switching.

## Capabilities and Constraints

- The menu-bar panel alone uses native UI. Settings and logs use the browser.
- Local request history exposes request and response content, status, token usage,
  time to first token and generation speed, with search and raw data downloads.
- Provider CRUD, credentials, logging, common settings and complete YAML editing.
- Request examples for OpenAI Chat Completions, Responses and Claude Messages,
  with editable parameters, streaming and copyable curl commands.
- Store account keys as literal config values; preserve unknown YAML fields.
- No credential environment lookup, .env files, variable references or environment profiles.
- Service and account edits apply immediately to new requests without restarting.
- Other config edits require an explicit restart, or the next start when stopped.
- Management and historical logs remain accessible while the proxy is stopped.
- A single globally selected provider receives native protocol paths; provider selection is persisted and applies immediately to new requests. Reverse proxying is supported; forward proxy and MITM are not supported.
- Single active config; defaults live in the user's application data directory.
