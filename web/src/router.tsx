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

const rootRoute = createRootRoute({
  component: Outlet,
  validateSearch: (search: Record<string, unknown>) => search,
  notFoundComponent: () => <p>页面不存在。</p>,
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
