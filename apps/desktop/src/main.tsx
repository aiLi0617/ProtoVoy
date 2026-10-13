import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@protovoy/ui/styles.css";
import "@protovoy/ui/curl-import.css";
import { ProtoVoyProviders } from "@protovoy/ui";
import { bootstrapClientStorage } from "@protovoy/app-wiring";
import { App } from "./App";

bootstrapClientStorage();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ProtoVoyProviders><App /></ProtoVoyProviders>
  </StrictMode>,
);
