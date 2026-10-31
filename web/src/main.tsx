import React from "react";
import { initializeLocale } from "./i18n";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import { createAppRouter } from "./router";
import "./styles.css";

const router = createAppRouter();
void initializeLocale();

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>,
);
