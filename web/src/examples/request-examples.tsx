import LanguageSelect from "../language-select";
import { t, useLocale } from "../i18n";
import ServiceToolbar from "../service-toolbar";
import { useEffect, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Textarea } from "../components/ui/textarea";
import { SegmentedControl } from "../components/ui/segmented-control";
import { Field, Toggle } from "../settings/settings-controls";
import "./request-examples.css";

const protocols = {
  chat: {
    label: "OpenAI Chat Completions",
    path: "chat/completions",
    base: "http://127.0.0.1:5601/v1",
    docs: "https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create",
  },
  responses: {
    label: "OpenAI Responses",
    path: "responses",
    base: "http://127.0.0.1:5601/v1",
    docs: "https://developers.openai.com/api/reference/resources/responses/methods/create",
  },
  claude: {
    label: "Claude Messages",
    path: "messages",
    base: "http://127.0.0.1:5601/v1",
    docs: "https://platform.claude.com/docs/en/api/messages/create",
  },
};
type Protocol = keyof typeof protocols;

function requestURL(protocol: Protocol, base: string) {
  return base.trim().replace(/\/+$/, "") + "/" + protocols[protocol].path;
}

// POSIX shell quoting preserves JSON and user input, including apostrophes.
function quote(value: string) {
  return "'" + value.replaceAll("'", "'\"'\"'") + "'";
}
export function buildCurl(
  protocol: Protocol,
  base: string,
  model: string,
  prompt: string,
  stream: boolean,
) {
  const body = {
    model,
    ...(protocol === "claude" ? { max_tokens: 1024 } : {}),
    ...(protocol === "responses"
      ? { input: prompt }
      : { messages: [{ role: "user", content: prompt }] }),
    stream,
  };
  return [
    t(
      "# Set TOKENHUB_API_KEY in your terminal: use client_key or the upstream API key",
    ),
    `curl${stream ? " --no-buffer" : ""} ${quote(requestURL(protocol, base))} \\`,
    "  -H 'Content-Type: application/json' \\",
    protocol === "claude"
      ? '  -H "x-api-key: ${TOKENHUB_API_KEY:-tokenhub}" \\'
      : '  -H "Authorization: Bearer ${TOKENHUB_API_KEY:-tokenhub}" \\',
    ...(protocol === "claude"
      ? ["  -H 'anthropic-version: 2023-06-01' \\"]
      : []),
    `  --data-raw ${quote(JSON.stringify(body, null, 2))}`,
  ].join("\n");
}
export default function RequestExamples() {
  const locale = useLocale();
  useEffect(() => {
    document.title = t("Request examples · TokenHub");
  }, [locale]);
  const [protocol, setProtocol] = useState<Protocol>("chat");
  const [base, setBase] = useState(protocols.chat.base);
  const [model, setModel] = useState("YOUR_MODEL_ID");
  const [prompt, setPrompt] = useState(
    t("Hello, please briefly introduce yourself."),
  );
  const [stream, setStream] = useState(false);
  const [notice, setNotice] = useState("");
  const [copyError, setCopyError] = useState("");
  const curl = buildCurl(protocol, base, model, prompt, stream);
  useEffect(() => {}, []);
  useEffect(() => {
    setNotice("");
    setCopyError("");
  }, [curl]);
  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(() => setNotice(""), 2000);
    return () => clearTimeout(timer);
  }, [notice]);
  return (
    <div className="app-shell">
      <header className="topbar">
        <Link className="brand" to="/">
          TokenHub
        </Link>
        <Link className="logs-link" to="/logs">
          {t("Back to request logs")}
        </Link>
        <LanguageSelect />
        <ServiceToolbar />
      </header>
      <main className="request-examples">
        <h1>{t("Request examples")}</h1>
        <p className="intro">
          {t(
            "Choose a protocol and enter the request parameters. Copy the curl command to your terminal, then check the request logs for results.",
          )}
        </p>
        <section
          aria-label={t("Request parameters")}
          className="example-fields"
        >
          <Field label={t("Protocol")}>
            <SegmentedControl
              options={Object.entries(protocols).map(([value, protocol]) => ({
                value,
                label: protocol.label,
              }))}
              aria-label={t("Protocol")}
              value={protocol}
              onValueChange={(value) => {
                const next = value as Protocol;
                if (base === protocols[protocol].base)
                  setBase(protocols[next].base);
                setProtocol(next);
              }}
            />
          </Field>
          <Field
            label={t("API base URL")}
            help={t(
              "Enter the proxy address and protocol version path, e.g. http://127.0.0.1:5601/v1. Port 5173 is for the frontend, not model requests. Do not add a provider name.",
            )}
          >
            <Input
              value={base}
              onChange={(event) => setBase(event.target.value)}
              spellCheck={false}
            />
          </Field>
          <Field
            label={t("Model")}
            help={t(
              "Enter the full model ID supported by the target service, including the vendor prefix if required.",
            )}
          >
            <Input
              value={model}
              onChange={(event) => setModel(event.target.value)}
              spellCheck={false}
            />
          </Field>
          <Field label={t("Prompt")}>
            <Textarea
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              rows={3}
            />
          </Field>
          <Field label={t("Response mode")} inputId="example-stream">
            <Toggle
              id="example-stream"
              label={t("Streaming response (SSE)")}
              value={stream}
              onChange={setStream}
            />
          </Field>
        </section>
        <section aria-labelledby="curl-heading" className="example-code">
          <div className="example-code-header">
            <h2 id="curl-heading">curl</h2>
            <code>POST {requestURL(protocol, base)}</code>
            <Button
              variant="outline"
              size="sm"
              disabled={!base.trim() || !model.trim() || !prompt.trim()}
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(curl);
                  setNotice(t("Copied"));
                  setCopyError("");
                } catch {
                  setCopyError(
                    t(
                      "Could not copy. Select the code below and copy it manually.",
                    ),
                  );
                }
              }}
            >
              {t("Copy curl")}
            </Button>
          </div>
          <pre tabIndex={0} aria-label={t("curl command")}>
            <code>{curl}</code>
          </pre>
          <span role="status">{notice}</span>
          {copyError && <p role="alert">{copyError}</p>}
        </section>
        <p className="intro">
          {t(
            "TokenHub forwards requests without converting protocols. The target service must support the selected endpoint. Set backend_key to inject an upstream key. When client_key is configured, set TOKENHUB_API_KEY in your terminal. Otherwise, use the upstream API key.",
          )}
        </p>
        <a href={protocols[protocol].docs} target="_blank" rel="noreferrer">
          {t("View {0} documentation", [protocols[protocol].label])}
        </a>
      </main>
    </div>
  );
}
