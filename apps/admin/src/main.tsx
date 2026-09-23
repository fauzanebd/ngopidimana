import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "../../../packages/theme/theme.css";
import "./styles.css";

// A magic link lands on /auth/callback?token=…; the token is read once here and the path is
// normalised to / before anything renders, so the token never survives in history, and a reload
// of the callback URL falls through to the ordinary session lookup instead of reporting a bad link.
function callbackToken() {
  if (window.location.pathname !== "/auth/callback") return null;
  const token = new URLSearchParams(window.location.search).get("token") || "";
  window.history.replaceState(null, "", "/");
  return token || null;
}

ReactDOM.createRoot(document.getElementById("root")!).render(<React.StrictMode><App callbackToken={callbackToken()} /></React.StrictMode>);
