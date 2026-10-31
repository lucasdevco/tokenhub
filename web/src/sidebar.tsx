import LanguageSelect from "./language-select";
import { t, useLocale, type MessageKey } from "./i18n";
import {
  Activity,
  Terminal,
  FileCode2,
  Settings2,
  SlidersHorizontal,
  Waypoints,
} from "lucide-react";
import { Button } from "./components/ui/button";
import { Link } from "@tanstack/react-router";

export const settingsItems = [
  ["basic", "General settings"],
  ["providers", "Provider"],
  ["yaml", "Full YAML"],
  ["app", "App preferences"],
] as const satisfies readonly (readonly [string, MessageKey])[];

export type SettingsTab = (typeof settingsItems)[number][0];

export function settingsTab(path: string): SettingsTab {
  return (
    settingsItems.find(([key]) => key === path.split("/")[2])?.[0] ?? "basic"
  );
}

const icons = {
  basic: Settings2,
  providers: Waypoints,
  yaml: FileCode2,
  app: SlidersHorizontal,
};

export default function Sidebar({
  active,
  settingsAvailable = true,
}: {
  active: SettingsTab | "logs";
  settingsAvailable?: boolean;
}) {
  useLocale();
  return (
    <aside>
      <nav aria-label={t("Main navigation")}>
        <Button asChild variant="ghost" className="nav-item">
          <Link
            to="/logs"
            className={active === "logs" ? "selected" : ""}
            aria-current={active === "logs" ? "page" : undefined}
          >
            <Activity aria-hidden="true" />
            {t("Request logs")}
          </Link>
        </Button>
        {settingsAvailable &&
          settingsItems
            .filter(([key]) => key !== "yaml")
            .map(([key, label]) => {
              const Icon = icons[key];
              return (
                <Button key={key} asChild variant="ghost" className="nav-item">
                  <Link
                    key={key}
                    to={`/settings/${key}`}
                    className={active === key ? "selected" : ""}
                    aria-current={active === key ? "page" : undefined}
                  >
                    <Icon aria-hidden="true" />
                    {t(label)}
                  </Link>
                </Button>
              );
            })}
        {settingsAvailable && (
          <details
            className="sidebar-advanced"
            open={active === "yaml" ? true : undefined}
          >
            <summary>{t("Advanced")}</summary>
            {settingsItems
              .filter(([key]) => key === "yaml")
              .map(([key, label]) => {
                const Icon = icons[key];
                return (
                  <Button
                    key={key}
                    asChild
                    variant="ghost"
                    className="nav-item"
                  >
                    <Link
                      to={`/settings/${key}`}
                      className={active === key ? "selected" : ""}
                      aria-current={active === key ? "page" : undefined}
                    >
                      <Icon aria-hidden="true" />
                      {t(label)}
                    </Link>
                  </Button>
                );
              })}
          </details>
        )}
        <LanguageSelect />
        <Button asChild variant="ghost" className="nav-item sidebar-bottom">
          <Link to="/examples">
            <Terminal aria-hidden="true" />
            {t("Request examples")}
          </Link>
        </Button>
      </nav>
    </aside>
  );
}
