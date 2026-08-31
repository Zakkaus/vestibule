import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "./i18n";
import { App } from "./app/App";
import "./styles/tokens.css";
import "./styles/components.css";
import "./styles/shell.css";
import "./app/app.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);
