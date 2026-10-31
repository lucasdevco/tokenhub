import { t, useLocale } from "./i18n";
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
  useLocale();
  return (
    <div className="topbar-controls" aria-label={t("Runtime configuration")}>
      <label className="topbar-switch" title={providerHelp}>
        <Waypoints aria-hidden="true" />
        <span className="topbar-control-label">Provider</span>
        <NativeSelect
          aria-label={t("Current provider")}
          title={providerHelp}
          size="sm"
          value={activeProvider}
          disabled={locked || switchingLocked || providerLocked}
          onChange={(event) => onProviderChange(event.target.value)}
        >
          <option value="">{t("No provider selected")}</option>
          {activeProvider && !providers.includes(activeProvider) && (
            <option value={activeProvider}>
              {activeProvider}
              {t(" (running)")}
            </option>
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
            ? t("Save or discard your changes before switching accounts.")
            : t("Switch accounts. New requests use the selection immediately.")
        }
      >
        <KeyRound aria-hidden="true" />
        <span className="topbar-control-label">{t("Account")}</span>
        <NativeSelect
          aria-label={t("Current account")}
          size="sm"
          value={activeAccount}
          disabled={
            locked || !accounts.length || switchingLocked || providerLocked
          }
          onChange={(event) => onAccountChange(event.target.value)}
        >
          {!accounts.length && <option value="">{t("No key required")}</option>}
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
        title={
          pending
            ? t("Restart and apply saved configuration")
            : t("Restart service")
        }
      >
        <RotateCw
          aria-hidden="true"
          className={restarting ? "restart-spinning" : ""}
        />
        {restarting ? t("Restarting…") : t("Restart")}
      </Button>
    </div>
  );
}
