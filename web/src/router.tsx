import { t, useLocale } from "./i18n";
import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
  type RouterHistory,
  useLocation,
} from "@tanstack/react-router";
import ProviderSettings from "./settings/provider-settings";
import Settings, { SettingsPage } from "./settings/settings";
import Logs from "./logs/logs";
import RequestExamples from "./examples/request-examples";
import { settingsItems } from "./sidebar";

function NotFound() {
  useLocale();
  return <p>{t("Page not found.")}</p>;
}

const rootRoute = createRootRoute({
  component: Outlet,
  validateSearch: (search: Record<string, unknown>) => search,
  notFoundComponent: NotFound,
});
const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: Logs,
});
const logsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "logs",
  component: Logs,
});
function SettingsEntry() {
  useLocale();
  const providersPage = useLocation({
    select: (location) => location.pathname === "/settings/providers",
  });
  return providersPage ? <ProviderSettings /> : <Settings />;
}

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "settings",
  component: SettingsEntry,
});
const examplesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "examples",
  component: RequestExamples,
});
const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/settings/basic", replace: true });
  },
});
const settingsRoutes = settingsItems.map(([tab]) =>
  createRoute({
    getParentRoute: () => settingsRoute,
    path: tab,
    component: () => <SettingsPage tab={tab} />,
  }),
);
const routeTree = rootRoute.addChildren([
  homeRoute,
  logsRoute,
  examplesRoute,
  settingsRoute.addChildren([settingsIndexRoute, ...settingsRoutes]),
]);

export function createAppRouter(history?: RouterHistory) {
  return createRouter({ routeTree, history });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}
