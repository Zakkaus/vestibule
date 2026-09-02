import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "./i18n";
// The mark is a placeholder. shield-check is borrowed from the vendored Lucide
// set until a real one exists, and it is referenced rather than copied so the
// icon manifest stays the only place those bytes live.
import placeholderMark from "./icons/lucide/shield-check.svg?url";
import { App } from "./app/App";
import "./styles/tokens.css";
import "./styles/components.css";
import "./styles/shell.css";
import "./app/app.css";

const icon = document.createElement("link");
icon.rel = "icon";
icon.type = "image/svg+xml";
icon.href = placeholderMark;
document.head.append(icon);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);
