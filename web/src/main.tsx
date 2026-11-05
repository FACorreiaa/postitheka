import "@fontsource-variable/schibsted-grotesk";
import "@fontsource-variable/schibsted-grotesk/wght-italic.css";
import "./styles.css";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
