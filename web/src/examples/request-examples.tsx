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
    "# 在终端设置 TOKENHUB_API_KEY：使用 client_key 或上游 API Key",
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
  const [protocol, setProtocol] = useState<Protocol>("chat");
  const [base, setBase] = useState(protocols.chat.base);
  const [model, setModel] = useState("YOUR_MODEL_ID");
  const [prompt, setPrompt] = useState("你好，请简单介绍一下自己。");
  const [stream, setStream] = useState(false);
  const [notice, setNotice] = useState("");
  const [copyError, setCopyError] = useState("");
  const curl = buildCurl(protocol, base, model, prompt, stream);
  useEffect(() => {
    document.title = "请求示例 · TokenHub";
  }, []);
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
          返回请求日志
        </Link>
        <ServiceToolbar />
      </header>
      <main className="request-examples">
        <h1>请求示例</h1>
        <p className="intro">
          选择协议并填写请求参数，复制 curl
          到终端发送请求，再到请求日志查看结果。
        </p>
        <section aria-label="请求参数" className="example-fields">
          <Field label="协议">
            <SegmentedControl
              options={Object.entries(protocols).map(([value, protocol]) => ({
                value,
                label: protocol.label,
              }))}
              aria-label="协议"
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
            label="API 基础地址"
            help="填写代理监听地址与协议版本路径，例如 http://127.0.0.1:5601/v1。5173 是前端端口，不用于模型请求；无需添加 Provider 名称。"
          >
            <Input
              value={base}
              onChange={(event) => setBase(event.target.value)}
              spellCheck={false}
            />
          </Field>
          <Field
            label="模型"
            help="填写目标服务支持的完整模型 ID，必要时包含供应商前缀。"
          >
            <Input
              value={model}
              onChange={(event) => setModel(event.target.value)}
              spellCheck={false}
            />
          </Field>
          <Field label="提示词">
            <Textarea
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              rows={3}
            />
          </Field>
          <Field label="响应方式" inputId="example-stream">
            <Toggle
              id="example-stream"
              label="流式响应（SSE）"
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
                  setNotice("已复制");
                  setCopyError("");
                } catch {
                  setCopyError("复制失败，请选中下方代码手动复制。");
                }
              }}
            >
              复制 curl
            </Button>
          </div>
          <pre tabIndex={0} aria-label="curl 请求代码">
            <code>{curl}</code>
          </pre>
          <span role="status">{notice}</span>
          {copyError && <p role="alert">{copyError}</p>}
        </section>
        <p className="intro">
          TokenHub 只转发请求，不转换协议。目标服务必须支持所选接口；配置
          backend_key 后由代理注入上游密钥，配置 client_key 时需在终端设置
          TOKENHUB_API_KEY；未配置密钥替换时填写上游 API Key。
        </p>
        <a href={protocols[protocol].docs} target="_blank" rel="noreferrer">
          查看 {protocols[protocol].label} 官方文档
        </a>
      </main>
    </div>
  );
}
