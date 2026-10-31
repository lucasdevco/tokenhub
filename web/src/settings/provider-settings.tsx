import { t, useLocale } from "../i18n";
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
const serviceMarks: Record<string, string> = {
  openrouter: "OR",
  openai: "OA",
  anthropic: "A",
};
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
  placeholder = t("No key required"),
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
}) {
  const locale = useLocale();
  useEffect(() => {
    document.title = t("Providers · TokenHub");
  }, [locale]);
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
  useLocale();
  return (
    <span className={`pa-mark pa-mark-${kind}`} aria-hidden="true">
      {serviceMarks[kind] || <Waypoints size={20} />}
    </span>
  );
}
export default function ProviderSettings() {
  useLocale();
  const [catalog, setCatalog] = useState<Catalog>();
  const [busy, setBusy] = useState(false);
  const services = catalog?.services || [];
  const activeId = catalog?.active || "";
  const [editor, setEditor] = useState<Service | null>(null);
  const [baseline, setBaseline] = useState("");
  const [adding, setAdding] = useState(false);
  const [accountEditor, setAccountEditor] = useState<Account | null>(null);
  const initialAccountName = useRef("");
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
      setNotice(t("Service restarted. Saved configuration applied."));
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
    } finally {
      setBusy(false);
    }
  }
  useBlocker({
    enableBeforeUnload: dirty,
    shouldBlockFn: () => dirty && !confirm(t("Discard unsaved changes?")),
  });
  function open(service: Service) {
    if (dirty && !confirm(t("Discard unsaved changes?"))) return;
    setEditor(structuredClone(service));
    setBaseline(JSON.stringify(service));
    setAdding(false);
    setAccountEditor(null);
    setShowKey(false);
    setError("");
    setDeleting(false);
  }
  function create() {
    if (busy || (dirty && !confirm(t("Discard unsaved changes?")))) return;
    const service: Service = {
      id: crypto.randomUUID(),
      name: "",
      kind: "custom",
      address: "",
      accounts: [],
      active: "",
      header: "Authorization",
      template: "Bearer {}",
      logging: true,
      proxy_enabled: true,
    };
    setEditor(service);
    setBaseline(JSON.stringify(service));
    setAdding(true);
    initialAccountName.current = t("Personal account");
    setAccountEditor({
      id: crypto.randomUUID(),
      name: initialAccountName.current,
      key: "",
    });
    setShowKey(false);
    setError("");
    setDeleting(false);
    setAddToCurrent(true);
  }
  function close() {
    if (busy) return;
    if (dirty && !confirm(t("Discard unsaved changes?"))) return;
    setEditor(null);
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
      accountEditor.name === initialAccountName.current
    )
      return editor;
    const account = {
      ...accountEditor,
      name: accountEditor.name.trim(),
      key: accountEditor.key.trim(),
    };
    if (!account.name || !account.key) {
      setError(t("Enter an account name and API key."));
      return null;
    }
    if (
      editor.accounts.some(
        (item) => item.id !== account.id && item.name === account.name,
      )
    ) {
      setError(t("This account name already exists. Choose another name."));
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
      setError(t("Enter a provider name and service URL."));
      return;
    }
    try {
      const url = new URL(next.address);
      if (!["http:", "https:"].includes(url.protocol)) throw Error();
    } catch {
      setError(t("Enter a complete URL starting with https:// or http://."));
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
        t("{0} {1}. New requests will use the saved settings.", [
          adding ? t("Added") : t("Saved"),
          next.name,
        ]),
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
      t("Switched to {0}.", [service.name]),
    );
  }
  async function switchAccount(id: string) {
    if (!current) return;
    const changed = await change(
      { action: "account", provider: current.id, account: id },
      t("Switched to {0}. New requests use it immediately.", [
        current.accounts.find((account) => account.id === id)?.name || "",
      ]),
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
        t("Deleted {0}.", [editor.name]),
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
              ? t("Running")
              : catalog.status.state === "error"
                ? t("Failed to start")
                : t("Stopped")}
          </span>
        )}
        <div className="pa-header-actions">
          <Button
            variant="ghost"
            size="sm"
            disabled={busy || dirty}
            onClick={reload}
          >
            {t("Reload")}
          </Button>
          {catalog?.status.pending && (
            <Button size="sm" disabled={busy || dirty} onClick={restart}>
              {t("Restart and apply")}
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
              {t(
                'Other configuration changes are pending. Click "Restart and apply" before editing providers or switching accounts.',
              )}
            </p>
          )}
          {!catalog ? (
            <div className="empty">
              {error
                ? t("Could not load providers. Please reload.")
                : t("Loading providers…")}
            </div>
          ) : (
            <>
              <div className="heading">
                <div>
                  <h1>{t("Provider")}</h1>
                  <p className="intro">
                    {t(
                      "Manage service URLs and accounts, and switch between accounts.",
                    )}
                  </p>
                </div>
                <Button
                  size="sm"
                  disabled={busy || catalog.status.pending}
                  onClick={create}
                >
                  <Plus />
                  {t("Add provider")}
                </Button>
              </div>
              <div className="pa-current">
                <div className="pa-current-title">
                  <span className="pa-dot" />
                  <span>{t("In use")}</span>
                </div>
                <div className="pa-switch">
                  <span>{t("Provider")}</span>
                  <Picker
                    label={t("Switch provider")}
                    disabled={busy || dirty || catalog.status.pending}
                    placeholder={t("No service provider selected")}
                    value={activeId}
                    options={services.map((service) => ({
                      value: service.id,
                      label: service.name,
                    }))}
                    onChange={switchService}
                  />
                  <span>{t("Account")}</span>
                  <Picker
                    label={t("Switch account")}
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
              <p className="pa-switch-help">
                {t("Each provider remembers the last account you used.")}
              </p>
              <div className="pa-workspace">
                <div className="pa-list">
                  <div className="pa-list-heading">
                    <h2>
                      {t("Configured providers")}
                      <span>{services.length}</span>
                    </h2>
                    <Input
                      className="pa-search"
                      aria-label={t("Search providers")}
                      placeholder={t("Search name, URL, or account")}
                      value={search}
                      onChange={(event) => setSearch(event.target.value)}
                    />
                  </div>
                  <div className="pa-services">
                    {visible.map((service) => (
                      <button
                        className={`pa-service-row${editor?.id === service.id ? " selected" : ""}`}
                        aria-pressed={editor?.id === service.id}
                        key={service.id}
                        onClick={() => open(service)}
                        aria-label={t("Edit {0}", [service.name])}
                        disabled={busy || catalog.status.pending}
                      >
                        <ServiceMark kind={service.kind} />
                        <span className="pa-service-info">
                          <strong>
                            {service.name}
                            {service.id === activeId && (
                              <span className="pa-using">{t("In use")}</span>
                            )}
                          </strong>
                          <small>{service.address}</small>
                        </span>
                        <span className="pa-account-summary">
                          <span>
                            {service.accounts.find(
                              (account) => account.id === service.active,
                            )?.name || t("No key required")}
                          </span>
                          <small>
                            {service.accounts.length
                              ? t("{0} accounts", [service.accounts.length])
                              : t("No accounts")}
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
                            ? t("No providers found")
                            : t("Add your first provider")}
                        </strong>
                        <p>
                          {services.length
                            ? t("Try another name, URL, or account.")
                            : t(
                                "Enter a service name, URL, and API key to get started.",
                              )}
                        </p>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() =>
                            services.length ? setSearch("") : create()
                          }
                        >
                          {services.length
                            ? t("Clear search")
                            : t("Add provider")}
                        </Button>
                      </div>
                    )}
                  </div>
                  <p className="pa-list-help">
                    {t("Select a provider to edit its URL or manage accounts.")}
                  </p>
                </div>
                <section
                  className="pa-editor"
                  aria-labelledby="pa-editor-title"
                >
                  <div className="pa-dialog-heading">
                    <h2 id="pa-editor-title">
                      {adding
                        ? t("Add provider")
                        : editor?.name || t("Provider settings")}
                    </h2>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={t("Close editor")}
                      onClick={close}
                    >
                      <X />
                    </Button>
                  </div>
                  <p className="pa-dialog-description">
                    {adding
                      ? t(
                          "Enter a name, URL, and API key for your custom service.",
                        )
                      : editor
                        ? t("Manage this provider's URL and accounts.")
                        : t(
                            "Select a provider on the left to edit its URL or manage accounts.",
                          )}
                  </p>
                  {editor && (
                    <form
                      onSubmit={(event) => {
                        event.preventDefault();
                        save();
                      }}
                    >
                      <fieldset disabled={busy} className="pa-dialog-fields">
                        <div className="pa-dialog-body">
                          <Field label={t("Name")}>
                            <Input
                              autoFocus={adding}
                              value={editor.name}
                              placeholder={t("e.g. OpenRouter")}
                              onChange={(event) =>
                                patch({ name: event.target.value })
                              }
                            />
                          </Field>
                          <Field label={t("Service URL")}>
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
                              {t("Account")}
                              <span>{editor.accounts.length}</span>
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
                                {t("Add account")}
                              </Button>
                            )}
                          </div>
                          <div className="pa-accounts">
                            {editor.accounts.map((account) => (
                              <div className="pa-account-row" key={account.id}>
                                <KeyRound size={16} />
                                <div>
                                  <strong>{account.name}</strong>
                                  <small>{t("API key configured")}</small>
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
                                      ? t("Current account")
                                      : t("Use this account")}
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
                                  {t("Edit")}
                                </Button>
                              </div>
                            ))}
                            {!editor.accounts.length && !accountEditor && (
                              <p className="pa-no-accounts">
                                {t(
                                  "No accounts yet. Local services can work without a key.",
                                )}
                              </p>
                            )}
                          </div>
                          {accountEditor && (
                            <div className="pa-account-form">
                              <h3>
                                {editor.accounts.some(
                                  (account) => account.id === accountEditor.id,
                                )
                                  ? t("Edit account")
                                  : t("New account")}
                              </h3>
                              <Field label={t("Account name")}>
                                <Input
                                  autoFocus={!adding}
                                  value={accountEditor.name}
                                  placeholder={t("e.g. Personal, Work")}
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
                                    placeholder={t("Paste your API key")}
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
                                        ? t("Hide API key")
                                        : t("Show API key")
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
                                  (account) => account.id === accountEditor.id,
                                ) && (
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    className="danger"
                                    onClick={() => {
                                      if (
                                        !confirm(
                                          t(
                                            'Delete account "{0}"? This takes effect after saving the provider.',
                                            [accountEditor.name],
                                          ),
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
                                    {t("Delete account")}
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
                                  {t("Cancel")}
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
                                  {t("Done")}
                                </Button>
                              </div>
                            </div>
                          )}
                          <details className="pa-advanced">
                            <summary>
                              {t("Advanced settings")}
                              <ChevronDown size={14} />
                            </summary>
                            <Field label={t("Authorization header")}>
                              <Input
                                value={editor.header}
                                onChange={(event) =>
                                  patch({ header: event.target.value })
                                }
                              />
                            </Field>
                            <Field label={t("Key format")}>
                              <Input
                                value={editor.template}
                                onChange={(event) =>
                                  patch({ template: event.target.value })
                                }
                              />
                            </Field>
                            <Field
                              label={t("Upstream proxy")}
                              help={t(
                                "When enabled, use the upstream proxy in General settings. Otherwise, connect directly to the provider.",
                              )}
                            >
                              <Toggle
                                label={t("Enable upstream proxy")}
                                value={editor.proxy_enabled}
                                onChange={(value) =>
                                  patch({ proxy_enabled: value })
                                }
                              />
                            </Field>
                            <Field label={t("Request logs")}>
                              <Toggle
                                label={t("Record requests for this provider")}
                                value={editor.logging}
                                onChange={(value) => patch({ logging: value })}
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
                              {t("Use this provider after adding")}
                            </label>
                          )}
                          {error && (
                            <p className="pa-error" role="alert">
                              {error}
                            </p>
                          )}
                          {deleting && (
                            <div className="pa-delete-confirm" role="alert">
                              <p>
                                {t("Delete {0} and all its accounts?", [
                                  editor.name,
                                ])}
                              </p>
                              <div>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => setDeleting(false)}
                                >
                                  {t("Cancel")}
                                </Button>
                                <Button
                                  type="button"
                                  variant="destructive"
                                  size="sm"
                                  onClick={remove}
                                >
                                  {t("Confirm deletion")}
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
                              {t("Delete provider")}
                            </Button>
                          )}
                          <span />
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={close}
                          >
                            {t("Cancel")}
                          </Button>
                          <Button type="submit" size="sm">
                            {busy
                              ? t("Saving…")
                              : adding
                                ? t("Add provider")
                                : t("Saved")}
                          </Button>
                        </div>
                      </fieldset>
                    </form>
                  )}

                  {!editor && (
                    <div className="pa-empty">
                      <Waypoints size={28} />
                      <strong>{t("Select a provider")}</strong>
                      <p>
                        {t("Select a provider on the left, or add a new one.")}
                      </p>
                    </div>
                  )}
                </section>
              </div>
              <div className="pa-notice" role="status">
                {notice ||
                  t("Selected: {0}{1}", [
                    current?.name || t("No service provider selected"),
                    currentAccount ? ` · ${currentAccount.name}` : "",
                  ])}
              </div>
              <p className="pa-local-note">
                {t(
                  "Switching affects new requests only. Requests already in progress will complete.",
                )}
              </p>
            </>
          )}
        </main>
      </div>
    </div>
  );
}
