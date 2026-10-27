import { useEffect, useRef, useState } from "react";
import {
  Check,
  ChevronDown,
  ChevronRight,
  Eye,
  EyeOff,
  KeyRound,
  Plus,
  Trash2,
  Waypoints,
  X,
} from "lucide-react";
import { Link, useBlocker } from "@tanstack/react-router";
import Sidebar from "../sidebar";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Field, Toggle } from "./settings-controls";
import { request, setSession } from "./settings-api";
import "./provider-settings.css";

type Account = { id: string; name: string; key: string };
type Service = {
  id: string;
  name: string;
  address: string;
  kind: string;
  accounts: Account[];
  active: string;
  header: string;
  template: string;
  logging: boolean;
  proxy_enabled: boolean;
};
const presets = [
  {
    id: "openrouter",
    name: "OpenRouter",
    address: "https://openrouter.ai/api",
    mark: "OR",
  },
  {
    id: "openai",
    name: "OpenAI",
    address: "https://api.openai.com",
    mark: "OA",
  },
  {
    id: "anthropic",
    name: "Anthropic",
    address: "https://api.anthropic.com",
    mark: "A",
  },
  { id: "custom", name: "自定义服务", address: "", mark: "" },
];
type Catalog = {
  services: Service[];
  active: string;
  version: string;
  status: { state: string; pending: boolean; address: string; error?: string };
};
function Picker({
  label,
  value,
  options,
  onChange,
  disabled = false,
  placeholder = "无需密钥",
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    function outside(event: PointerEvent) {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, [open]);
  return (
    <div
      className="pa-picker"
      ref={root}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.stopPropagation();
          setOpen(false);
          trigger.current?.focus();
        }
        if (
          open &&
          ["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)
        ) {
          event.preventDefault();
          const items = Array.from(
            root.current!.querySelectorAll<HTMLButtonElement>(
              '[role="menuitemradio"]',
            ),
          );
          const current = items.indexOf(
            document.activeElement as HTMLButtonElement,
          );
          const next =
            event.key === "Home"
              ? 0
              : event.key === "End"
                ? items.length - 1
                : (current +
                    (event.key === "ArrowUp" ? -1 : 1) +
                    items.length) %
                  items.length;
          items[next]?.focus();
        }
      }}
    >
      <Button
        ref={trigger}
        variant="outline"
        size="sm"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        disabled={disabled || !options.length}
        onClick={() => setOpen(!open)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" && !open) {
            event.preventDefault();
            setOpen(true);
          }
        }}
      >
        {options.find((option) => option.value === value)?.label || placeholder}
        <ChevronDown />
      </Button>
      {open && (
        <div className="pa-menu" role="menu" aria-label={label}>
          {options.map((option) => (
            <button
              key={option.value}
              role="menuitemradio"
              aria-checked={value === option.value}
              onClick={() => {
                onChange(option.value);
                setOpen(false);
                trigger.current?.focus();
              }}
            >
              <span>{option.label}</span>
              {value === option.value && <Check size={14} />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
function ServiceMark({ kind }: { kind: string }) {
  return (
    <span className={`pa-mark pa-mark-${kind}`} aria-hidden="true">
      {presets.find((preset) => preset.id === kind)?.mark || (
        <Waypoints size={20} />
      )}
    </span>
  );
}
export default function ProviderSettings() {
  const [catalog, setCatalog] = useState<Catalog>();
  const [busy, setBusy] = useState(false);
  const services = catalog?.services || [];
  const activeId = catalog?.active || "";
  const [editor, setEditor] = useState<Service | null>(null);
  const [baseline, setBaseline] = useState("");
  const [adding, setAdding] = useState(false);
  const [choosing, setChoosing] = useState(false);
  const [accountEditor, setAccountEditor] = useState<Account | null>(null);
  const [showKey, setShowKey] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [search, setSearch] = useState("");
  const [addToCurrent, setAddToCurrent] = useState(false);
  const current = services.find((service) => service.id === activeId);
  const currentAccount = current?.accounts.find(
    (account) => account.id === current.active,
  );
  const dirty =
    !!editor && (JSON.stringify(editor) !== baseline || accountEditor !== null);
  const visible = services.filter((service) =>
    `${service.name} ${service.address} ${service.accounts.map((account) => account.name).join(" ")}`
      .toLowerCase()
      .includes(search.toLowerCase()),
  );
  useEffect(() => {
    document.title = "服务商 · TokenHub";
    let cancelled = false;
    async function initialize() {
      try {
        setSession((await request<{ token: string }>("/api/session")).token);
        const result = await request<Catalog>("/api/providers");
        if (!cancelled) setCatalog(result);
      } catch (value) {
        if (!cancelled)
          setError(value instanceof Error ? value.message : String(value));
      }
    }
    void initialize();
    const timer = setInterval(() => {
      request<Catalog["status"]>("/api/status")
        .then((status) => {
          if (!cancelled)
            setCatalog((current) =>
              current ? { ...current, status } : current,
            );
        })
        .catch(() => {});
    }, 3000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, []);
  async function reload() {
    setBusy(true);
    setError("");
    try {
      setSession((await request<{ token: string }>("/api/session")).token);
      setCatalog(await request<Catalog>("/api/providers"));
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
    } finally {
      setBusy(false);
    }
  }
  async function change(body: Record<string, unknown>, message: string) {
    if (!catalog || busy) return false;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await request<Catalog>("/api/providers", {
        ...body,
        version: catalog.version,
      });
      setCatalog(result);
      setNotice(message);
      return true;
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
      return false;
    } finally {
      setBusy(false);
    }
  }
  async function restart() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await request("/api/service/restart", {});
      setCatalog(await request<Catalog>("/api/providers"));
      setNotice("服务已重启，已应用保存的配置。");
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
    } finally {
      setBusy(false);
    }
  }
  useBlocker({
    enableBeforeUnload: dirty,
    shouldBlockFn: () => dirty && !confirm("放弃未保存的修改？"),
  });
  function open(service: Service) {
    if (dirty && !confirm("放弃未保存的修改？")) return;
    setEditor(structuredClone(service));
    setBaseline(JSON.stringify(service));
    setAdding(false);
    setChoosing(false);
    setAccountEditor(null);
    setShowKey(false);
    setError("");
    setDeleting(false);
  }
  function choose() {
    if (busy || (dirty && !confirm("放弃未保存的修改？"))) return;
    setEditor(null);
    setAccountEditor(null);
    setChoosing(true);
    setError("");
  }
  function create(kind: string) {
    const preset = presets.find((item) => item.id === kind)!;
    const service: Service = {
      id: crypto.randomUUID(),
      name: kind === "custom" ? "" : preset.name,
      kind,
      address: preset.address,
      accounts: [],
      active: "",
      header: kind === "anthropic" ? "x-api-key" : "Authorization",
      template: kind === "anthropic" ? "{}" : "Bearer {}",
      logging: true,
      proxy_enabled: true,
    };
    setEditor(service);
    setBaseline(JSON.stringify(service));
    setChoosing(false);
    setAdding(true);
    setAccountEditor({ id: crypto.randomUUID(), name: "个人账号", key: "" });
    setShowKey(false);
    setError("");
    setDeleting(false);
    setAddToCurrent(true);
  }
  function close() {
    if (busy) return;
    if (dirty && !confirm("放弃未保存的修改？")) return;
    setEditor(null);
    setChoosing(false);
    setAccountEditor(null);
    setError("");
  }
  function patch(values: Partial<Service>) {
    if (editor) setEditor({ ...editor, ...values });
    setError("");
  }
  function applyAccount(): Service | null {
    if (!editor || !accountEditor) return editor;
    if (
      adding &&
      editor.accounts.length === 0 &&
      accountEditor.key.trim() === "" &&
      accountEditor.name === "个人账号"
    )
      return editor;
    const account = {
      ...accountEditor,
      name: accountEditor.name.trim(),
      key: accountEditor.key.trim(),
    };
    if (!account.name || !account.key) {
      setError("请填写账号名称和 API Key。");
      return null;
    }
    if (
      editor.accounts.some(
        (item) => item.id !== account.id && item.name === account.name,
      )
    ) {
      setError("账号名称已存在，请换一个名称。");
      return null;
    }
    const exists = editor.accounts.some((item) => item.id === account.id);
    return {
      ...editor,
      accounts: exists
        ? editor.accounts.map((item) =>
            item.id === account.id ? account : item,
          )
        : [...editor.accounts, account],
      active: editor.active || account.id,
    };
  }
  async function save() {
    const next = applyAccount();
    if (!next) return;
    if (!next.name.trim() || !next.address.trim()) {
      setError("请填写服务商名称和服务地址。");
      return;
    }
    try {
      const url = new URL(next.address);
      if (!["http:", "https:"].includes(url.protocol)) throw Error();
    } catch {
      setError("请输入以 https:// 或 http:// 开头的完整地址。");
      return;
    }
    next.name = next.name.trim();
    next.address = next.address.trim();
    if (
      await change(
        {
          action: "save",
          service: next,
          new: adding,
          activate: adding && addToCurrent,
        },
        `已${adding ? "添加" : "保存"} ${next.name}，新请求将使用保存后的设置。`,
      )
    ) {
      setEditor(next);
      setBaseline(JSON.stringify(next));
      setAdding(false);
      setAccountEditor(null);
      setShowKey(false);
    }
  }
  async function switchService(id: string) {
    const service = services.find((item) => item.id === id)!;
    await change(
      { action: "select", provider: id },
      `已切换到 ${service.name}。`,
    );
  }
  async function switchAccount(id: string) {
    if (!current) return;
    const changed = await change(
      { action: "account", provider: current.id, account: id },
      `已切换到 ${current.accounts.find((account) => account.id === id)?.name}，新请求立即生效。`,
    );
    if (changed && editor?.id === current.id) {
      const next = { ...editor, active: id };
      setEditor(next);
      setBaseline(JSON.stringify(next));
    }
  }
  async function remove() {
    if (!editor) return;
    if (
      await change(
        { action: "delete", provider: editor.id },
        `已删除 ${editor.name}。`,
      )
    ) {
      setEditor(null);
      setAccountEditor(null);
    }
  }
  return (
    <div className="provider-settings">
      <header className="topbar">
        <Link className="brand" to="/settings/providers">
          TokenHub
        </Link>
        {catalog && (
          <span className={`status ${catalog.status.state}`}>
            <i />
            {catalog.status.state === "running"
              ? "运行中"
              : catalog.status.state === "error"
                ? "启动失败"
                : "已停止"}
          </span>
        )}
        <div className="pa-header-actions">
          <Button
            variant="ghost"
            size="sm"
            disabled={busy || dirty}
            onClick={reload}
          >
            重新加载
          </Button>
          {catalog?.status.pending && (
            <Button size="sm" disabled={busy || dirty} onClick={restart}>
              重启并应用
            </Button>
          )}
        </div>
      </header>
      <div className="layout">
        <Sidebar active="providers" />
        <main className="pa-main">
          {error && !editor && (
            <p className="pa-error" role="alert">
              {error}
            </p>
          )}
          {catalog?.status.pending && (
            <p className="pa-error" role="status">
              其他配置待应用，请先点击“重启并应用”，再修改服务商或切换账号。
            </p>
          )}
          {!catalog ? (
            <div className="empty">
              {error ? "无法加载服务商。请重新加载。" : "正在加载服务商…"}
            </div>
          ) : (
            <>
              <div className="heading">
                <div>
                  <h1>服务商</h1>
                  <p className="intro">
                    管理服务地址和账号，在不同账号之间轻松切换。
                  </p>
                </div>
                <Button
                  size="sm"
                  disabled={busy || catalog.status.pending}
                  onClick={() => {
                    choose();
                  }}
                >
                  <Plus />
                  添加服务商
                </Button>
              </div>
              <div className="pa-current">
                <div className="pa-current-title">
                  <span className="pa-dot" />
                  <span>当前使用</span>
                </div>
                <div className="pa-switch">
                  <span>服务商</span>
                  <Picker
                    label="切换服务商"
                    disabled={busy || dirty || catalog.status.pending}
                    placeholder="未选择服务商"
                    value={activeId}
                    options={services.map((service) => ({
                      value: service.id,
                      label: service.name,
                    }))}
                    onChange={switchService}
                  />
                  <span>账号</span>
                  <Picker
                    label="切换账号"
                    disabled={busy || dirty || catalog.status.pending}
                    value={current?.active || ""}
                    options={
                      current?.accounts.map((account) => ({
                        value: account.id,
                        label: account.name,
                      })) || []
                    }
                    onChange={switchAccount}
                  />
                </div>
              </div>
              <p className="pa-switch-help">每个服务商会记住上次使用的账号。</p>
              <div className="pa-workspace">
                <div className="pa-list">
                  <div className="pa-list-heading">
                    <h2>
                      已添加的服务商 <span>{services.length}</span>
                    </h2>
                    <Input
                      className="pa-search"
                      aria-label="搜索服务商"
                      placeholder="搜索名称、地址或账号"
                      value={search}
                      onChange={(event) => setSearch(event.target.value)}
                    />
                  </div>
                  <div className="pa-services">
                    {visible.map((service) => (
                      <button
                        className={`pa-service-row${!choosing && editor?.id === service.id ? " selected" : ""}`}
                        aria-pressed={!choosing && editor?.id === service.id}
                        key={service.id}
                        onClick={() => open(service)}
                        aria-label={`编辑 ${service.name}`}
                        disabled={busy || catalog.status.pending}
                      >
                        <ServiceMark kind={service.kind} />
                        <span className="pa-service-info">
                          <strong>
                            {service.name}
                            {service.id === activeId && (
                              <span className="pa-using">当前使用</span>
                            )}
                          </strong>
                          <small>{service.address}</small>
                        </span>
                        <span className="pa-account-summary">
                          <span>
                            {service.accounts.find(
                              (account) => account.id === service.active,
                            )?.name || "无需密钥"}
                          </span>
                          <small>
                            {service.accounts.length
                              ? `${service.accounts.length} 个账号`
                              : "未添加账号"}
                          </small>
                        </span>
                        <ChevronRight size={16} />
                      </button>
                    ))}
                    {!visible.length && (
                      <div className="pa-empty">
                        <Waypoints size={24} />
                        <strong>
                          {services.length
                            ? "没有找到服务商"
                            : "添加你的第一个服务商"}
                        </strong>
                        <p>
                          {services.length
                            ? "试试其他名称、地址或账号。"
                            : "选择服务商，填写 API Key 即可开始。"}
                        </p>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() =>
                            services.length ? setSearch("") : choose()
                          }
                        >
                          {services.length ? "清除搜索" : "添加服务商"}
                        </Button>
                      </div>
                    )}
                  </div>
                  <p className="pa-list-help">
                    点击服务商，编辑地址或管理它的账号。
                  </p>
                </div>
                <section
                  className="pa-editor"
                  aria-labelledby="pa-editor-title"
                >
                  <div className="pa-dialog-heading">
                    <h2 id="pa-editor-title">
                      {choosing || adding
                        ? "添加服务商"
                        : editor?.name || "服务商设置"}
                    </h2>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="关闭编辑"
                      onClick={close}
                    >
                      <X />
                    </Button>
                  </div>
                  <p className="pa-dialog-description">
                    {choosing
                      ? "选择服务商，常用地址会自动填写。"
                      : editor
                        ? "管理此服务商的地址和账号。"
                        : "从左侧选择服务商，编辑地址或管理账号。"}
                  </p>
                  {choosing ? (
                    <div className="pa-presets">
                      {presets.map((preset) => (
                        <button
                          className="pa-preset"
                          key={preset.id}
                          onClick={() => create(preset.id)}
                        >
                          <ServiceMark kind={preset.id} />
                          <span>
                            <strong>{preset.name}</strong>
                            <small>
                              {preset.id === "custom"
                                ? "其他兼容服务或本地服务"
                                : preset.address.replace("https://", "")}
                            </small>
                          </span>
                          <ChevronRight size={16} />
                        </button>
                      ))}
                    </div>
                  ) : (
                    editor && (
                      <form
                        onSubmit={(event) => {
                          event.preventDefault();
                          save();
                        }}
                      >
                        <fieldset disabled={busy} className="pa-dialog-fields">
                          <div className="pa-dialog-body">
                            <Field label="名称">
                              <Input
                                value={editor.name}
                                placeholder="例如：OpenRouter"
                                onChange={(event) =>
                                  patch({ name: event.target.value })
                                }
                              />
                            </Field>
                            <Field label="服务地址">
                              <Input
                                value={editor.address}
                                placeholder="https://api.example.com"
                                onChange={(event) =>
                                  patch({ address: event.target.value })
                                }
                              />
                            </Field>
                            <div className="pa-section-title">
                              <h2>
                                账号 <span>{editor.accounts.length}</span>
                              </h2>
                              {!accountEditor && (
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => {
                                    setAccountEditor({
                                      id: crypto.randomUUID(),
                                      name: "",
                                      key: "",
                                    });
                                    setShowKey(false);
                                    setError("");
                                  }}
                                >
                                  <Plus />
                                  添加账号
                                </Button>
                              )}
                            </div>
                            <div className="pa-accounts">
                              {editor.accounts.map((account) => (
                                <div
                                  className="pa-account-row"
                                  key={account.id}
                                >
                                  <KeyRound size={16} />
                                  <div>
                                    <strong>{account.name}</strong>
                                    <small>API Key 已设置</small>
                                  </div>
                                  <label className="pa-account-radio">
                                    <input
                                      type="radio"
                                      name="active-account"
                                      checked={editor.active === account.id}
                                      onChange={() =>
                                        patch({ active: account.id })
                                      }
                                    />
                                    <span>
                                      {editor.active === account.id
                                        ? "当前账号"
                                        : "使用此账号"}
                                    </span>
                                  </label>
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    disabled={!!accountEditor}
                                    onClick={() => {
                                      setAccountEditor({ ...account });
                                      setShowKey(false);
                                      setError("");
                                    }}
                                  >
                                    编辑
                                  </Button>
                                </div>
                              ))}
                              {!editor.accounts.length && !accountEditor && (
                                <p className="pa-no-accounts">
                                  还没有账号。本地服务可以不填密钥。
                                </p>
                              )}
                            </div>
                            {accountEditor && (
                              <div className="pa-account-form">
                                <h3>
                                  {editor.accounts.some(
                                    (account) =>
                                      account.id === accountEditor.id,
                                  )
                                    ? "编辑账号"
                                    : "新账号"}
                                </h3>
                                <Field label="账号名称">
                                  <Input
                                    autoFocus
                                    value={accountEditor.name}
                                    placeholder="例如：个人账号、公司账号"
                                    onChange={(event) => {
                                      setAccountEditor({
                                        ...accountEditor,
                                        name: event.target.value,
                                      });
                                      setError("");
                                    }}
                                  />
                                </Field>
                                <Field label="API Key" inputId="pa-api-key">
                                  <div className="pa-secret">
                                    <Input
                                      id="pa-api-key"
                                      type={showKey ? "text" : "password"}
                                      autoComplete="off"
                                      spellCheck={false}
                                      value={accountEditor.key}
                                      placeholder="粘贴你的 API Key"
                                      onChange={(event) => {
                                        setAccountEditor({
                                          ...accountEditor,
                                          key: event.target.value,
                                        });
                                        setError("");
                                      }}
                                    />
                                    <Button
                                      type="button"
                                      variant="ghost"
                                      size="sm"
                                      aria-label={
                                        showKey
                                          ? "隐藏 API Key"
                                          : "显示 API Key"
                                      }
                                      aria-pressed={showKey}
                                      onClick={() => setShowKey(!showKey)}
                                    >
                                      {showKey ? <EyeOff /> : <Eye />}
                                    </Button>
                                  </div>
                                </Field>
                                <div className="pa-account-form-actions">
                                  {editor.accounts.some(
                                    (account) =>
                                      account.id === accountEditor.id,
                                  ) && (
                                    <Button
                                      type="button"
                                      variant="ghost"
                                      size="sm"
                                      className="danger"
                                      onClick={() => {
                                        if (
                                          !confirm(
                                            `删除账号“${accountEditor.name}”？保存服务商后生效。`,
                                          )
                                        )
                                          return;
                                        const accounts = editor.accounts.filter(
                                          (account) =>
                                            account.id !== accountEditor.id,
                                        );
                                        patch({
                                          accounts,
                                          active:
                                            editor.active === accountEditor.id
                                              ? accounts[0]?.id || ""
                                              : editor.active,
                                        });
                                        setAccountEditor(null);
                                      }}
                                    >
                                      <Trash2 />
                                      删除账号
                                    </Button>
                                  )}
                                  <span />
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => {
                                      setAccountEditor(null);
                                      setError("");
                                    }}
                                  >
                                    取消
                                  </Button>
                                  <Button
                                    type="button"
                                    variant="outline"
                                    size="sm"
                                    onClick={() => {
                                      const next = applyAccount();
                                      if (next) {
                                        setEditor(next);
                                        setAccountEditor(null);
                                        setError("");
                                      }
                                    }}
                                  >
                                    完成
                                  </Button>
                                </div>
                              </div>
                            )}
                            <details className="pa-advanced">
                              <summary>
                                高级设置
                                <ChevronDown size={14} />
                              </summary>
                              <Field label="鉴权请求头">
                                <Input
                                  value={editor.header}
                                  onChange={(event) =>
                                    patch({ header: event.target.value })
                                  }
                                />
                              </Field>
                              <Field label="密钥格式">
                                <Input
                                  value={editor.template}
                                  onChange={(event) =>
                                    patch({ template: event.target.value })
                                  }
                                />
                              </Field>
                              <Field
                                label="上游代理"
                                help="开启时使用常用配置中的上游代理设置，关闭时直接连接服务商。"
                              >
                                <Toggle
                                  label="启用上游代理"
                                  value={editor.proxy_enabled}
                                  onChange={(value) =>
                                    patch({ proxy_enabled: value })
                                  }
                                />
                              </Field>
                              <Field label="请求日志">
                                <Toggle
                                  label="记录此服务商的请求"
                                  value={editor.logging}
                                  onChange={(value) =>
                                    patch({ logging: value })
                                  }
                                />
                              </Field>
                            </details>
                            {adding && (
                              <label className="pa-use-after">
                                <input
                                  type="checkbox"
                                  checked={addToCurrent}
                                  onChange={(event) =>
                                    setAddToCurrent(event.target.checked)
                                  }
                                />
                                添加后使用此服务商
                              </label>
                            )}
                            {error && (
                              <p className="pa-error" role="alert">
                                {error}
                              </p>
                            )}
                            {deleting && (
                              <div className="pa-delete-confirm" role="alert">
                                <p>删除 {editor.name} 及其所有账号？</p>
                                <div>
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => setDeleting(false)}
                                  >
                                    取消
                                  </Button>
                                  <Button
                                    type="button"
                                    variant="destructive"
                                    size="sm"
                                    onClick={remove}
                                  >
                                    确认删除
                                  </Button>
                                </div>
                              </div>
                            )}
                          </div>
                          <div className="pa-dialog-footer">
                            {!adding && (
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="danger"
                                onClick={() => setDeleting(true)}
                              >
                                <Trash2 />
                                删除服务商
                              </Button>
                            )}
                            <span />
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={close}
                            >
                              取消
                            </Button>
                            <Button type="submit" size="sm">
                              {busy
                                ? "保存中…"
                                : adding
                                  ? "添加服务商"
                                  : "保存"}
                            </Button>
                          </div>
                        </fieldset>
                      </form>
                    )
                  )}

                  {!choosing && !editor && (
                    <div className="pa-empty">
                      <Waypoints size={28} />
                      <strong>选择一个服务商</strong>
                      <p>在左侧选择服务商，或添加新的服务商。</p>
                    </div>
                  )}
                </section>
              </div>
              <div className="pa-notice" role="status">
                {notice ||
                  `当前选择：${current?.name || "未选择服务商"}${currentAccount ? ` · ${currentAccount.name}` : ""}`}
              </div>
              <p className="pa-local-note">
                切换只影响新请求，正在进行的请求会继续完成。
              </p>
            </>
          )}
        </main>
      </div>
    </div>
  );
}
