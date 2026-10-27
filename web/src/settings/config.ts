import { parseDocument } from "yaml";

export type Provider = {
  name?: string;
  kind?: string;
  accounts?: { id: string; name: string; key: string }[];
  active_account?: string;
  base_url?: string;
  logging?: boolean;
  proxy_enabled?: boolean;
  authorization?: {
    header?: string;
    template?: string;
    backend_key?: string;
    client_key?: string;
  };
};

// Form fields are typed; the YAML document still preserves unknown fields.
export type Configuration = {
  server?: { host?: string; port?: number };
  logging?: { enabled?: boolean; log_dir?: string; console?: boolean };
  http_client?: { proxy_url?: string; proxy_from_environment?: boolean };
  providers?: Record<string, Provider>;
  active_provider?: string;
  [key: string]: unknown;
};

// Conversion can throw even when YAML parsing succeeded (e.g. unresolved aliases).
export function readDraft(text: string) {
  const document = parseDocument(text);
  let config: Configuration | null = null;
  let error = document.errors.map((value) => value.message).join("\n");
  if (!error) {
    try {
      config = document.toJS();
      if (!config || typeof config !== "object" || Array.isArray(config)) {
        config = null;
        error = "配置根节点必须是 YAML 映射。";
      }
    } catch (value) {
      error = value instanceof Error ? value.message : String(value);
    }
  }
  return { document, config, error };
}

export function updateCredential(
  text: string,
  provider: string,
  key: string,
  value: string,
) {
  const document = parseDocument(text);
  const path = ["providers", provider, "authorization", key];
  if (value === "") document.deleteIn(path);
  else document.setIn(path, value);
  if (
    !document.getIn(["providers", provider, "authorization", "backend_key"]) &&
    !document.getIn(["providers", provider, "authorization", "client_key"])
  )
    document.deleteIn(["providers", provider, "authorization"]);
  return document.toString();
}
