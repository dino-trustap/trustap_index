import React from "react";
import ReactDOM from "react-dom/client";
import { AuthProvider } from "react-oidc-context";
import App from "./App.jsx";
import "./styles.css";

// Bootstrap: the backend tells us whether SSO is configured (mirrors the
// partners bridge pattern: react-oidc-context + PKCE against Trustap
// Keycloak). Without SSO config the dashboard runs open (dev mode).
async function bootstrap() {
  let config = { keycloak: { configured: false } };
  try {
    const res = await fetch("/api/dashboard/config");
    if (res.ok) config = await res.json();
  } catch {
    // Backend down; App will surface the error on data load.
  }

  const root = ReactDOM.createRoot(document.getElementById("root"));

  if (config.keycloak?.configured) {
    const oidcConfig = {
      authority: config.keycloak.authority,
      client_id: config.keycloak.client_id,
      redirect_uri: `${window.location.origin}/dashboard/openid/auth`,
      post_logout_redirect_uri: `${window.location.origin}/dashboard/`,
      scope: "openid profile email",
      onSigninCallback: () => {
        window.history.replaceState({}, document.title, "/dashboard/");
      },
    };
    root.render(
      <AuthProvider {...oidcConfig}>
        <App ssoEnabled />
      </AuthProvider>
    );
  } else {
    root.render(<App ssoEnabled={false} />);
  }
}

bootstrap();
