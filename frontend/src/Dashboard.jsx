import React, { useCallback, useEffect, useState } from "react";

const AGENTS_META = {
  chatgpt: { short: "GPT", label: "ChatGPT", vendor: "OpenAI" },
  copilot: { short: "CP", label: "Copilot", vendor: "Microsoft" },
  google: { short: "G", label: "Google", vendor: "Gemini / AI Mode" },
  perplexity: { short: "PX", label: "Perplexity", vendor: "Comet / Shopping" },
};

const ICONS = {
  signal: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M2 13.5v-4M6 13.5v-7M10 13.5v-10M14 13.5v-5.5"/></svg>,
  grid: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6"><rect x="2" y="2" width="5" height="5" rx="1"/><rect x="9" y="2" width="5" height="5" rx="1"/><rect x="2" y="9" width="5" height="5" rx="1"/><rect x="9" y="9" width="5" height="5" rx="1"/></svg>,
  link: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M6.5 9.5l3-3M5 11l-1.2 1.2a2.5 2.5 0 01-3.5-3.5L2.5 7M11 5l1.2-1.2a2.5 2.5 0 113.5 3.5L13.5 9" transform="translate(-1 0)"/></svg>,
  receipt: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M3 2h10v12l-1.7-1.2L9.6 14l-1.6-1.2L6.4 14l-1.7-1.2L3 14z"/><path d="M5.5 5.5h5M5.5 8h5"/></svg>,
  lock: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6"><rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 015 0v2"/></svg>,
};

const ISSUE_LABELS = {
  missing_image: "No image",
  missing_description: "No description",
  short_description: "Short description",
  missing_gtin: "No GTIN",
  missing_brand: "No brand",
  missing_category: "No category",
  out_of_stock: "Out of stock",
};

const ISSUE_HINTS = {
  missing_image: "All agents need a product image; Perplexity and Google will not list without one.",
  missing_description: "Agents rank products by description quality.",
  short_description: "Longer descriptions improve agent answers and ranking.",
  missing_gtin: "Perplexity effectively requires a GTIN; Google needs it (or brand + MPN).",
  missing_brand: "Used by Google and Perplexity for product matching.",
  missing_category: "Helps agents categorise the product.",
  out_of_stock: "Out-of-stock products are marked unavailable in every feed.",
};

function timeAgo(iso) {
  if (!iso) return "never";
  const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function money(minor, currency) {
  return `${(minor / 100).toFixed(2)} ${(currency || "eur").toUpperCase()}`;
}

const NAV = [
  { id: "agents", label: "Agent reach", icon: "signal" },
  { id: "catalog", label: "Catalog health", icon: "grid" },
  { id: "connections", label: "Connections", icon: "link" },
  { id: "orders", label: "Orders", icon: "receipt" },
];

export default function Dashboard({ token, userName, onLogout, devMode }) {
  const [merchants, setMerchants] = useState([]);
  const [merchantId, setMerchantId] = useState(null);
  const [overview, setOverview] = useState(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(true);
  const [updatedAt, setUpdatedAt] = useState(null);

  const apiFetch = useCallback(
    async (path) => {
      const headers = token ? { Authorization: `Bearer ${token}` } : {};
      const res = await fetch(path, { headers });
      if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
      return res.json();
    },
    [token]
  );

  useEffect(() => {
    apiFetch("/api/dashboard/merchants")
      .then((data) => {
        const list = data.merchants || [];
        setMerchants(list);
        if (list.length > 0) setMerchantId((current) => current || list[0].id);
        setLoading(false);
      })
      .catch((e) => {
        setError(e.message);
        setLoading(false);
      });
  }, [apiFetch]);

  const loadOverview = useCallback(() => {
    if (!merchantId) return;
    apiFetch(`/api/dashboard/merchants/${merchantId}/overview`)
      .then((data) => {
        setOverview(data);
        setUpdatedAt(new Date());
        setError(null);
      })
      .catch((e) => setError(e.message));
  }, [apiFetch, merchantId]);

  useEffect(() => {
    loadOverview();
    const interval = setInterval(loadOverview, 30000);
    return () => clearInterval(interval);
  }, [loadOverview]);

  if (loading) {
    return (
      <div className="fullscreen">
        <div className="logo-plate"><img src="/dashboard/trustap-logo.png" alt="Trustap" className="fullscreen-logo" /></div>
        <div className="loader" />
      </div>
    );
  }

  const health = overview?.catalog_health;
  const summary = health?.summary;
  const connection = overview?.connection;
  const activeAgents = (overview?.agents || []).filter((a) => a.status === "active").length;
  const merchantName = merchants.find((m) => m.id === merchantId)?.name || merchantId;

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="logo-plate"><img src="/dashboard/trustap-logo.png" alt="Trustap" className="sidebar-logo" /></div>
        <div className="sidebar-product">INDEX</div>

        {merchants.length > 1 ? (
          <select
            className="merchant-select"
            value={merchantId || ""}
            onChange={(e) => setMerchantId(e.target.value)}
          >
            {merchants.map((m) => (
              <option key={m.id} value={m.id}>{m.name}</option>
            ))}
          </select>
        ) : (
          <div className="merchant-static">{merchantName}</div>
        )}

        <nav className="sidebar-nav">
          {NAV.map((n) => (
            <a key={n.id} href={`#${n.id}`}>
              <span className="nav-icon">{ICONS[n.icon]}</span>
              {n.label}
            </a>
          ))}
        </nav>

        <div className="sidebar-foot">
          {devMode && <span className="pill pill-amber">dev mode</span>}
          {userName && <div className="user-name" title={userName}>{userName}</div>}
          {onLogout && (
            <button className="btn btn-ghost btn-small" onClick={onLogout}>Log out</button>
          )}
          <div className="secure-note">
            {ICONS.lock}
            <span>Payments protected by Trustap buyer protection</span>
          </div>
        </div>
      </aside>

      <main className="content">
        {error && <div className="banner banner-error">{error}</div>}

        {overview && (
          <>
            <header className="hero">
              <div>
                <h1>Overview</h1>
                <p className="hero-sub">
                  {merchantName} · {activeAgents > 0
                    ? `${activeAgents} of ${overview.agents.length} AI agents pulled your catalog in the last 24 hours`
                    : "catalog live, waiting for the first agent fetch"}
                </p>
              </div>
              <div className="hero-side">
                <span className="live-dot" />
                <span>updated {updatedAt ? timeAgo(updatedAt.toISOString()) : ""}</span>
                <button className="btn btn-ghost btn-small" onClick={loadOverview}>Refresh</button>
              </div>
            </header>

            <section className="stat-row">
              <Stat label="Products live" value={summary?.total ?? 0} />
              <Stat
                label="Ready for all agents"
                value={`${summary?.ready_all ?? 0}/${summary?.total ?? 0}`}
                tone={summary && summary.ready_all < summary.total ? "amber" : "green"}
              />
              <Stat label="Paid orders" value={connection?.payments?.paid_count ?? 0} />
              <Stat label="Revenue" value={money(connection?.payments?.revenue_minor ?? 0, "eur")} tone="primary" />
            </section>

            <section id="agents" className="section">
              <h2>Agent reach</h2>
              <p className="section-sub">Whether each AI shopping agent can reach your catalog, and when it last did.</p>
              <div className="agent-grid">
                {(overview.agents || []).map((a) => <AgentCard key={a.key} agent={a} />)}
              </div>
            </section>

            <section id="catalog" className="section">
              <div className="section-head">
                <div>
                  <h2>Catalog health</h2>
                  <p className="section-sub">Products that need extra information to be listed well by every agent.</p>
                </div>
                {summary && summary.total > 0 && (
                  <ReadinessRing ready={summary.ready_all} total={summary.total} />
                )}
              </div>
              <CatalogTable health={health} />
            </section>

            <section id="connections" className="section">
              <h2>Connections</h2>
              <div className="conn-grid">
                <ConnCard
                  title="Trustap payments"
                  ok={connection?.trustap?.connected}
                  okText="Connected"
                  badText="Not connected"
                  detail={connection?.trustap?.connected
                    ? "Checkouts settle through your Trustap account with buyer protection."
                    : "Add your Trustap API credentials to start selling."}
                />
                <ConnCard
                  title="Payment notifications"
                  ok={!!connection?.webhooks?.last_event}
                  okText={`Last event ${timeAgo(connection?.webhooks?.last_event)}`}
                  badText="No events yet"
                  detail="Trustap notifies the Index when buyers pay; orders and stock update automatically."
                />
                <ConnCard
                  title="Store sync"
                  ok={connection?.store_sync?.status === "connected"}
                  okText="Synced"
                  badText="Not connected"
                  detail={connection?.store_sync?.note}
                  action={<button className="btn btn-disabled" disabled>Connect your store (coming soon)</button>}
                />
              </div>
            </section>

            <section id="orders" className="section">
              <h2>Recent orders</h2>
              <CheckoutsTable checkouts={overview.recent_checkouts || []} />
            </section>

            <footer className="content-foot">
              Trustap Index · one catalog, every AI shopping agent
            </footer>
          </>
        )}
      </main>
    </div>
  );
}

function Stat({ label, value, tone }) {
  return (
    <div className={`stat ${tone ? `stat-tone-${tone}` : ""}`}>
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  );
}

function AgentCard({ agent }) {
  const meta = AGENTS_META[agent.key] || { short: "?", label: agent.name, vendor: "" };
  const statusMeta = {
    active: { label: "Active", cls: "pill-green", hint: "Fetched your catalog in the last 24h", pulse: true },
    quiet: { label: "Quiet", cls: "pill-amber", hint: "Has fetched before, nothing in the last 24h" },
    waiting: { label: "Ready", cls: "pill-gray", hint: "Surfaces are live, waiting for first fetch" },
  }[agent.status] || { label: agent.status, cls: "pill-gray" };

  return (
    <div className={`card agent-card ${agent.status === "active" ? "is-active" : ""}`}>
      <div className="agent-head">
        <span className="agent-avatar">{meta.short}</span>
        <div className="agent-names">
          <span className="agent-name">{meta.label}</span>
          <span className="agent-vendor">{meta.vendor}</span>
        </div>
        <span className={`pill ${statusMeta.cls}`} title={statusMeta.hint}>
          {statusMeta.pulse && <span className="pulse" />}
          {statusMeta.label}
        </span>
      </div>
      <div className="agent-meta">
        <div>
          <div className="agent-metric">{timeAgo(agent.last_fetch)}</div>
          <div className="agent-metric-label">last fetch</div>
        </div>
        <div>
          <div className="agent-metric">{agent.hits_24h}</div>
          <div className="agent-metric-label">fetches / 24h</div>
        </div>
      </div>
      <div className="agent-links">
        {(agent.surfaces || []).map((s) => (
          <a key={s.surface} href={s.url} target="_blank" rel="noreferrer" title={s.url}>
            {s.surface.replace(/_/g, " ")} ↗
          </a>
        ))}
      </div>
    </div>
  );
}

function ReadinessRing({ ready, total }) {
  const pct = total > 0 ? Math.round((ready / total) * 100) : 0;
  const r = 26;
  const c = 2 * Math.PI * r;
  return (
    <div className="ring" title={`${ready} of ${total} products are fully ready for every agent`}>
      <svg viewBox="0 0 64 64" width="64" height="64">
        <circle cx="32" cy="32" r={r} fill="none" className="ring-track" strokeWidth="7" />
        <circle
          cx="32" cy="32" r={r} fill="none"
          stroke={pct === 100 ? "#34d399" : "#4d65ff"}
          strokeWidth="7" strokeLinecap="round"
          strokeDasharray={`${(pct / 100) * c} ${c}`}
          transform="rotate(-90 32 32)"
        />
        <text x="32" y="37" textAnchor="middle" className="ring-text">{pct}%</text>
      </svg>
      <span className="ring-label">agent ready</span>
    </div>
  );
}

function CatalogTable({ health }) {
  const products = health?.products || [];
  if (products.length === 0) {
    return <div className="card empty">No products yet. Add products via the API to appear in agent catalogs.</div>;
  }
  return (
    <div className="card table-card">
      <table>
        <thead>
          <tr>
            <th>Product</th>
            <th>Price</th>
            <th>Stock</th>
            <th>Agent readiness</th>
            <th>Needs attention</th>
          </tr>
        </thead>
        <tbody>
          {products.map((p) => (
            <tr key={p.id}>
              <td className="td-title">{p.title}</td>
              <td className="td-nowrap">{money(p.price, p.currency)}</td>
              <td className={p.quantity <= 0 ? "td-bad" : ""}>{p.quantity}</td>
              <td className="td-nowrap">
                {Object.entries(AGENTS_META).map(([k, meta]) => (
                  <span
                    key={k}
                    className={`mini-chip ${p.readiness?.[k] ? "mini-ok" : "mini-bad"}`}
                    title={`${meta.label}: ${p.readiness?.[k] ? "listed with full data" : "missing data"}`}
                  >
                    {meta.short}
                  </span>
                ))}
              </td>
              <td>
                {(p.issues || []).length === 0 ? (
                  <span className="pill pill-green">All good</span>
                ) : (
                  (p.issues || []).map((iss) => (
                    <span key={iss} className="chip" title={ISSUE_HINTS[iss] || iss}>
                      {ISSUE_LABELS[iss] || iss}
                    </span>
                  ))
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ConnCard({ title, ok, okText, badText, detail, action }) {
  return (
    <div className="card conn-card">
      <div className="conn-head">
        <span className="conn-title">{title}</span>
        <span className={`pill ${ok ? "pill-green" : "pill-gray"}`}>{ok ? okText : badText}</span>
      </div>
      <p className="conn-detail">{detail}</p>
      {action}
    </div>
  );
}

function CheckoutsTable({ checkouts }) {
  if (checkouts.length === 0) {
    return <div className="card empty">No orders yet.</div>;
  }
  const statusCls = {
    paid: "pill-green",
    pending_payment: "pill-amber",
    cancelled: "pill-gray",
    failed: "pill-red",
  };
  return (
    <div className="card table-card">
      <table>
        <thead>
          <tr><th>Order</th><th>Amount</th><th>Status</th><th>Trustap tx</th><th>Created</th></tr>
        </thead>
        <tbody>
          {checkouts.map((c) => (
            <tr key={c.id}>
              <td className="td-title">{c.description}</td>
              <td className="td-nowrap">{money(c.price, c.currency)}</td>
              <td><span className={`pill ${statusCls[c.status] || "pill-gray"}`}>{c.status.replace(/_/g, " ")}</span></td>
              <td>{c.transaction_id || "-"}</td>
              <td className="td-nowrap">{timeAgo(c.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
