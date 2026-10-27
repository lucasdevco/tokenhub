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

Configure local AI API providers, control the proxy from the menu bar, and inspect
captured requests in the system browser.

## Capabilities and Constraints

- The menu-bar panel alone uses native UI. Settings and logs use the browser.
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
