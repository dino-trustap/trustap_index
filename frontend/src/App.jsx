import React from "react";
import { useAuth } from "react-oidc-context";
import Dashboard from "./Dashboard.jsx";

function SsoGate() {
  const auth = useAuth();

  if (auth.isLoading) {
    return <FullScreen message="Signing you in..." />;
  }
  if (auth.error) {
    return (
      <FullScreen
        message={`Login failed: ${auth.error.message}`}
        action={() => auth.signinRedirect()}
        actionLabel="Try again"
      />
    );
  }
  if (!auth.isAuthenticated) {
    return (
      <FullScreen
        message="Sign in with your Trustap account to manage your Index presence."
        action={() => auth.signinRedirect()}
        actionLabel="Sign in with Trustap"
        logo
      />
    );
  }

  return (
    <Dashboard
      token={auth.user?.id_token}
      userName={auth.user?.profile?.name || auth.user?.profile?.email}
      onLogout={() => auth.signoutRedirect()}
    />
  );
}

function FullScreen({ message, action, actionLabel, logo }) {
  return (
    <div className="fullscreen">
      {logo && <img src="/dashboard/trustap-logo.svg" alt="Trustap" className="fullscreen-logo" />}
      <p>{message}</p>
      {action && (
        <button className="btn btn-primary" onClick={action}>
          {actionLabel}
        </button>
      )}
    </div>
  );
}

export default function App({ ssoEnabled }) {
  if (ssoEnabled) return <SsoGate />;
  return <Dashboard token={null} userName={null} onLogout={null} devMode />;
}
