import { KeyRound, RotateCw, Waypoints } from "lucide-react";
import { NativeSelect } from "./components/ui/native-select";
import { Button } from "./components/ui/button";

type Props = {
  accounts: { id: string; name: string }[];
  activeAccount: string;
  providers: string[];
  providerLabels?: Record<string, string>;
  activeProvider: string;
  locked: boolean;
  switchingLocked?: boolean;
  providerLocked?: boolean;
  providerHelp: string;
  canRestart: boolean;
  pending?: boolean;
  restarting: boolean;
  onAccountChange: (account: string) => void;
  onProviderChange: (provider: string) => void;
  onRestart: () => void;
};

export function ServiceControls({
  accounts,
  activeAccount,
  providers,
  providerLabels = {},
  activeProvider,
  locked,
  switchingLocked = false,
  providerLocked = false,
  providerHelp,
  canRestart,
  pending,
  restarting,
  onAccountChange,
  onProviderChange,
  onRestart,
}: Props) {
  return (
    <div className="topbar-controls" aria-label="运行配置">
      <label className="topbar-switch" title={providerHelp}>
        <Waypoints aria-hidden="true" />
        <span className="topbar-control-label">Provider</span>
        <NativeSelect
          aria-label="当前 Provider"
          title={providerHelp}
          size="sm"
          value={activeProvider}
          disabled={locked || switchingLocked || providerLocked}
          onChange={(event) => onProviderChange(event.target.value)}
        >
          <option value="">未选择 Provider</option>
          {activeProvider && !providers.includes(activeProvider) && (
            <option value={activeProvider}>{activeProvider}（当前运行）</option>
          )}
          {providers.map((name) => (
            <option key={name} value={name}>
              {providerLabels[name] || name}
            </option>
          ))}
        </NativeSelect>
      </label>
      <label
        className="topbar-switch"
        title={
          switchingLocked
            ? "请先保存或放弃当前修改，再切换账号。"
            : "切换账号，新请求立即生效"
        }
      >
        <KeyRound aria-hidden="true" />
        <span className="topbar-control-label">账号</span>
        <NativeSelect
          aria-label="当前账号"
          size="sm"
          value={activeAccount}
          disabled={
            locked || !accounts.length || switchingLocked || providerLocked
          }
          onChange={(event) => onAccountChange(event.target.value)}
        >
          {!accounts.length && <option value="">无需密钥</option>}
          {accounts.map((account) => (
            <option key={account.id} value={account.id}>
              {account.name}
            </option>
          ))}
        </NativeSelect>
      </label>
      <Button
        variant="outline"
        size="sm"
        disabled={locked || !canRestart}
        onClick={onRestart}
        title={pending ? "重启并应用已保存的配置" : "重启服务"}
      >
        <RotateCw
          aria-hidden="true"
          className={restarting ? "restart-spinning" : ""}
        />
        {restarting ? "重启中…" : "重启"}
      </Button>
    </div>
  );
}
