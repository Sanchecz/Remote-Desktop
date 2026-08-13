import { Component, StrictMode, type ErrorInfo, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles.css";

function exposeFatalBootstrapError(reason: unknown) {
  window.setTimeout(() => {
    const root = document.getElementById("root");
    if (!root || root.childElementCount > 0) return;
    const message = reason instanceof Error ? reason.message : String(reason || "Неизвестная ошибка интерфейса");
    const screen = document.createElement("main");
    screen.className = "fatal-error-screen";
    screen.setAttribute("role", "alert");
    const title = document.createElement("strong");
    title.textContent = "RemoteIt не смог открыть этот экран";
    const details = document.createElement("span");
    details.textContent = message;
    screen.append(title, details);
    root.append(screen);
  }, 0);
}

window.addEventListener("error", (event) => exposeFatalBootstrapError(event.error || event.message));
window.addEventListener("unhandledrejection", (event) => exposeFatalBootstrapError(event.reason));

class RemoteItErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("RemoteIt interface failure", error, info.componentStack);
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <main className="fatal-error-screen" role="alert">
        <strong>RemoteIt не смог открыть этот экран</strong>
        <span>{this.state.error.message || "Неизвестная ошибка интерфейса"}</span>
        <button type="button" onClick={() => window.location.reload()}>Перезагрузить панель</button>
      </main>
    );
  }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <RemoteItErrorBoundary><App /></RemoteItErrorBoundary>
  </StrictMode>
);

if ("serviceWorker" in navigator && import.meta.env.PROD) {
  window.addEventListener("load", () => {
    void navigator.serviceWorker.register("/sw.js", { scope: "/" });
  });
}
