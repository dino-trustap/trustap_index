import React, { useCallback, useEffect, useState } from "react";

const AGENTS_META = {
  chatgpt: { short: "GPT", label: "ChatGPT", vendor: "OpenAI" },
  copilot: { short: "CP", label: "Copilot", vendor: "Microsoft" },
  google: { short: "G", label: "Google", vendor: "Gemini / AI Mode" },
  perplexity: { short: "PX", label: "Perplexity", vendor: "Comet / Shopping" },
};

const ICONS = {
  home: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M2.5 6.5L8 2l5.5 4.5V13a1 1 0 01-1 1h-3v-4h-3v4h-3a1 1 0 01-1-1z"/></svg>,
  signal: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><path d="M2 13.5v-4M6 13.5v-7M10 13.5v-10M14 13.5v-5.5"/></svg>,
  grid: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="2" y="2" width="5" height="5" rx="1"/><rect x="9" y="2" width="5" height="5" rx="1"/><rect x="2" y="9" width="5" height="5" rx="1"/><rect x="9" y="9" width="5" height="5" rx="1"/></svg>,
  link: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><path d="M5.5 8.5l5-5M4 12l-1 1a2.4 2.4 0 01-3.4-3.4L1 8.2M12 4l1-1a2.4 2.4 0 113.4 3.4L15 7.8" transform="translate(-0.5 0)"/></svg>,
  receipt: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M3 2h10v12l-1.7-1.2L9.6 14l-1.6-1.2L6.4 14l-1.7-1.2L3 14z"/><path d="M5.5 5.5h5M5.5 8h5"/></svg>,
  lock: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 015 0v2"/></svg>,
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

const PAGES = [
  { id: "overview", label: "Overview", icon: "home", sub: "How your catalog is doing across AI shopping agents." },
  { id: "agents", label: "Agent reach", icon: "signal", sub: "Every surface AI agents use to discover your products." },
  { id: "catalog", label: "Catalog", icon: "grid", sub: "Listing quality and the data each agent still needs." },
  { id: "connections", label: "Connections", icon: "link", sub: "Payments, notifications, and store synchronisation." },
  { id: "orders", label: "Orders", icon: "receipt", sub: "Checkouts created by agents and their payment state." },
];

function pageFromHash() {
  const h = window.location.hash.replace("#", "");
  return PAGES.some((p) => p.id === h) ? h : "overview";
}

export default function Dashboard({ token, userName, onLogout, devMode }) {
  const [merchants, setMerchants] = useState([]);
  const [merchantId, setMerchantId] = useState(null);
  const [overview, setOverview] = useState(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(true);
  const [updatedAt, setUpdatedAt] = useState(null);
  const [page, setPage] = useState(pageFromHash());

  const navigate = useCallback((id) => {
    setPage(id);
    window.location.hash = id;
    window.scrollTo({ top: 0 });
  }, []);

  useEffect(() => {
    const onHash = () => setPage(pageFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

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
        <img src="/dashboard/trustap-logo.png" alt="Trustap" className="fullscreen-logo" />
        <div className="loader" />
      </div>
    );
  }

  const merchantName = merchants.find((m) => m.id === merchantId)?.name || merchantId;
  const summary = overview?.catalog_health?.summary;
  const attention = summary ? summary.with_issues : 0;
  const meta = PAGES.find((p) => p.id === page) || PAGES[0];

  return (
    <div className="shell">
      <aside className="sidebar">
        <img src="/dashboard/trustap-logo.png" alt="Trustap" className="sidebar-logo" />
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
          {PAGES.map((p) => (
            <button
              key={p.id}
              className={page === p.id ? "active" : ""}
              onClick={() => navigate(p.id)}
            >
              <span className="nav-icon">{ICONS[p.icon]}</span>
              {p.label}
              {p.id === "catalog" && attention > 0 && <span className="nav-count">{attention}</span>}
            </button>
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
            <span>Payments protected by Trustap</span>
          </div>
        </div>
      </aside>

      <main className="content">
        {error && <div className="banner banner-error">{error}</div>}

        {overview && (
          <div className="page" key={page}>
            <header className="page-head">
              <div>
                <h1>{meta.label}</h1>
                <p className="page-sub">{meta.sub}</p>
              </div>
              <div className="page-side">
                <span className="live-dot" />
                <span>updated {updatedAt ? timeAgo(updatedAt.toISOString()) : ""}</span>
                <button className="btn btn-ghost btn-small" onClick={loadOverview}>Refresh</button>
              </div>
            </header>

            {page === "overview" && <OverviewPage overview={overview} navigate={navigate} />}
            {page === "agents" && <AgentsPage agents={overview.agents || []} />}
            {page === "catalog" && <CatalogPage health={overview.catalog_health} />}
            {page === "connections" && <ConnectionsPage connection={overview.connection} />}
            {page === "orders" && <OrdersPage checkouts={overview.recent_checkouts || []} />}
          </div>
        )}
      </main>
    </div>
  );
}

/* ---------------- overview ---------------- */

function OverviewPage({ overview, navigate }) {
  const summary = overview.catalog_health?.summary;
  const connection = overview.connection;
  const agents = overview.agents || [];
  const products = overview.catalog_health?.products || [];
  const needAttention = products.filter((p) => (p.issues || []).length > 0);
  const orders = (overview.recent_checkouts || []).slice(0, 5);

  return (
    <>
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

      <div className="grid-2">
        <div className="card">
          <h2>Agent activity</h2>
          {agents.map((a) => {
            const m = AGENTS_META[a.key] || { short: "?", label: a.name };
            return (
              <div className="list-row" key={a.key}>
                <span className="grow" style={{ fontWeight: 600 }}>{m.label}</span>
                <span className="dim">{a.hits_24h} fetches · {timeAgo(a.last_fetch)}</span>
                <StatusPill status={a.status} />
              </div>
            );
          })}
          <div style={{ marginTop: "0.7rem" }}>
            <button className="link-btn" onClick={() => navigate("agents")}>View agent surfaces →</button>
          </div>
        </div>

        <div className="card">
          <h2>Needs attention</h2>
          {needAttention.length === 0 ? (
            <p className="empty" style={{ padding: "0.8rem 0" }}>Every product is fully listed. Nothing to fix.</p>
          ) : (
            needAttention.slice(0, 5).map((p) => (
              <div className="list-row" key={p.id}>
                <span className="grow" style={{ fontWeight: 600 }}>{p.title}</span>
                <span className="dim">{(p.issues || []).length} issue{p.issues.length > 1 ? "s" : ""}</span>
                <span className="pill pill-amber">{ISSUE_LABELS[p.issues[0]] || p.issues[0]}</span>
              </div>
            ))
          )}
          {needAttention.length > 0 && (
            <div style={{ marginTop: "0.7rem" }}>
              <button className="link-btn" onClick={() => navigate("catalog")}>Open catalog →</button>
            </div>
          )}
        </div>
      </div>

      <div className="card" style={{ marginTop: "0.9rem" }}>
        <h2>Latest orders</h2>
        {orders.length === 0 ? (
          <p className="empty" style={{ padding: "0.8rem 0" }}>No orders yet.</p>
        ) : (
          orders.map((c) => (
            <div className="list-row" key={c.id}>
              <span className="grow" style={{ fontWeight: 600 }}>{c.description}</span>
              <span className="dim">{money(c.price, c.currency)}</span>
              <span className="dim">{timeAgo(c.created_at)}</span>
              <OrderPill status={c.status} />
            </div>
          ))
        )}
        <div style={{ marginTop: "0.7rem" }}>
          <button className="link-btn" onClick={() => navigate("orders")}>View all orders →</button>
        </div>
      </div>
    </>
  );
}

/* ---------------- agents ---------------- */

function AgentsPage({ agents }) {
  return (
    <div className="agent-grid">
      {agents.map((a) => <AgentCard key={a.key} agent={a} />)}
    </div>
  );
}

function AgentCard({ agent }) {
  const meta = AGENTS_META[agent.key] || { short: "?", label: agent.name, vendor: "" };
  return (
    <div className={`card ${agent.status === "active" ? "is-active" : ""}`}>
      <div className="agent-head">
        <span className="agent-avatar">{meta.short}</span>
        <div className="agent-names">
          <span className="agent-name">{meta.label}</span>
          <span className="agent-vendor">{meta.vendor}</span>
        </div>
        <StatusPill status={agent.status} />
      </div>
      <div className="agent-stats">
        <div>
          <div className="agent-metric">{timeAgo(agent.last_fetch)}</div>
          <div className="agent-metric-label">last fetch</div>
        </div>
        <div>
          <div className="agent-metric">{agent.hits_24h}</div>
          <div className="agent-metric-label">fetches / 24h</div>
        </div>
      </div>
      <div>
        {(agent.surfaces || []).map((s) => (
          <div className="surface-row" key={s.surface}>
            <span className="surface-name">{s.surface.replace(/_/g, " ")}</span>
            <span className="dim" style={{ fontSize: "0.74rem", color: "var(--text-3)" }}>
              {s.last_fetch ? timeAgo(s.last_fetch) : "never fetched"}
            </span>
            <a href={s.url} target="_blank" rel="noreferrer">open</a>
            <CopyButton text={s.url} />
          </div>
        ))}
      </div>
    </div>
  );
}

function CopyButton({ text }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      className={`copy-btn ${copied ? "copied" : ""}`}
      onClick={() => {
        navigator.clipboard.writeText(text).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1300);
        });
      }}
    >
      {copied ? "Copied" : "Copy URL"}
    </button>
  );
}

/* ---------------- catalog ---------------- */

function CatalogPage({ health }) {
  const [query, setQuery] = useState("");
  const products = health?.products || [];
  const summary = health?.summary;
  const filtered = products.filter((p) =>
    p.title.toLowerCase().includes(query.toLowerCase())
  );
  const pct = summary && summary.total > 0 ? Math.round((summary.ready_all / summary.total) * 100) : 0;

  return (
    <>
      <div className="progress-row">
        <span style={{ whiteSpace: "nowrap" }}>{pct}% agent ready</span>
        <div className="progress"><div style={{ width: `${pct}%` }} /></div>
        <input
          className="search"
          placeholder="Search products..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {filtered.length === 0 ? (
        <div className="card empty">
          {products.length === 0
            ? "No products yet. Add products via the API to appear in agent catalogs."
            : "No products match your search."}
        </div>
      ) : (
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
              {filtered.map((p) => (
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
                      <span className="pill pill-green">Complete</span>
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
      )}
    </>
  );
}

/* ---------------- connections ---------------- */

function ConnectionsPage({ connection }) {
  return (
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
  );
}

function ConnCard({ title, ok, okText, badText, detail, action }) {
  return (
    <div className="card">
      <div className="conn-head">
        <span className="conn-title">{title}</span>
        <span className={`pill ${ok ? "pill-green" : "pill-gray"}`}>{ok ? okText : badText}</span>
      </div>
      <p className="conn-detail">{detail}</p>
      {action}
    </div>
  );
}

/* ---------------- orders ---------------- */

function OrdersPage({ checkouts }) {
  if (checkouts.length === 0) {
    return <div className="card empty">No orders yet.</div>;
  }
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
              <td><OrderPill status={c.status} /></td>
              <td>{c.transaction_id || "-"}</td>
              <td className="td-nowrap">{timeAgo(c.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ---------------- shared ---------------- */

function Stat({ label, value, tone }) {
  return (
    <div className={`card stat ${tone ? `stat-tone-${tone}` : ""}`}>
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  );
}

function StatusPill({ status }) {
  const meta = {
    active: { label: "Active", cls: "pill-green", hint: "Fetched your catalog in the last 24h", pulse: true },
    quiet: { label: "Quiet", cls: "pill-amber", hint: "Has fetched before, nothing in the last 24h" },
    waiting: { label: "Ready", cls: "pill-gray", hint: "Surfaces are live, waiting for first fetch" },
  }[status] || { label: status, cls: "pill-gray" };
  return (
    <span className={`pill ${meta.cls}`} title={meta.hint}>
      {meta.pulse && <span className="pulse" />}
      {meta.label}
    </span>
  );
}

function OrderPill({ status }) {
  const cls = {
    paid: "pill-green",
    pending_payment: "pill-amber",
    cancelled: "pill-gray",
    failed: "pill-red",
  }[status] || "pill-gray";
  return <span className={`pill ${cls}`}>{status.replace(/_/g, " ")}</span>;
}
