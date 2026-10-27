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
  ["basic", "常用配置"],
  ["providers", "服务商"],
  ["yaml", "完整 YAML"],
  ["app", "客户端偏好"],
] as const;

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
  return (
    <aside>
      <nav aria-label="主导航">
        <Button asChild variant="ghost" className="nav-item">
          <Link
            to="/logs"
            className={active === "logs" ? "selected" : ""}
            aria-current={active === "logs" ? "page" : undefined}
          >
            <Activity aria-hidden="true" />
            请求日志
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
                    {label}
                  </Link>
                </Button>
              );
            })}
        {settingsAvailable && (
          <details
            className="sidebar-advanced"
            open={active === "yaml" ? true : undefined}
          >
            <summary>高级</summary>
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
                      {label}
                    </Link>
                  </Button>
                );
              })}
          </details>
        )}
        <Button asChild variant="ghost" className="nav-item sidebar-bottom">
          <Link to="/examples">
            <Terminal aria-hidden="true" />
            请求示例
          </Link>
        </Button>
      </nav>
    </aside>
  );
}
