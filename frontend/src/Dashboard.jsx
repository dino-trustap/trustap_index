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
  open: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M6.5 3.5H3.8A1.3 1.3 0 002.5 4.8v7.4a1.3 1.3 0 001.3 1.3h7.4a1.3 1.3 0 001.3-1.3V9.5M9.5 2.5h4v4M13 3L7.5 8.5"/></svg>,
  copy: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="5.5" y="5.5" width="8" height="8" rx="1.2"/><path d="M10.5 5.5v-2a1.2 1.2 0 00-1.2-1.2h-6a1.2 1.2 0 00-1.2 1.2v6a1.2 1.2 0 001.2 1.2h2"/></svg>,
  check: <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M3 8.5l3.2 3L13 4.5"/></svg>,
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

/* ---------------- charts ---------------- */

function Bars({ data, height = 30, barWidth = 7, gap = 3, color = "var(--primary)" }) {
  const max = Math.max(...data, 1);
  return (
    <svg
      className="sparkbars"
      width={data.length * (barWidth + gap) - gap}
      height={height}
      role="img"
      aria-label="activity over the last 24 hours"
    >
      {data.map((v, i) => {
        const h = v === 0 ? 2 : Math.max(3, Math.round((v / max) * height));
        return (
          <rect
            key={i}
            x={i * (barWidth + gap)}
            y={height - h}
            width={barWidth}
            height={h}
            rx="1.5"
            fill={v === 0 ? "var(--line)" : color}
          >
            <title>{`${v} fetches`}</title>
          </rect>
        );
      })}
    </svg>
  );
}

function AreaChart({ data, labels, height = 200, format }) {
  const [hover, setHover] = useState(null);
  const W = 760;
  const pad = { t: 16, r: 12, b: 26, l: 12 };
  const innerW = W - pad.l - pad.r;
  const innerH = height - pad.t - pad.b;
  const max = Math.max(...data, 1);
  const stepX = innerW / Math.max(data.length - 1, 1);
  const pts = data.map((v, i) => [pad.l + i * stepX, pad.t + innerH - (v / max) * innerH]);

  let line = "";
  pts.forEach(([x, y], i) => {
    if (i === 0) { line = `M${x},${y}`; return; }
    const [px, py] = pts[i - 1];
    const cx = (px + x) / 2;
    line += ` C${cx},${py} ${cx},${y} ${x},${y}`;
  });
  const baseline = pad.t + innerH;
  const area = `${line} L${pts[pts.length - 1][0]},${baseline} L${pts[0][0]},${baseline} Z`;

  const onMove = (e) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = ((e.clientX - rect.left) / rect.width) * W;
    const idx = Math.min(data.length - 1, Math.max(0, Math.round((x - pad.l) / stepX)));
    setHover(idx);
  };

  const h = hover;
  const tipW = 120;
  const tipX = h == null ? 0 : Math.min(Math.max(pts[h][0] - tipW / 2, pad.l), W - tipW - pad.r);

  return (
    <svg
      className="areachart"
      viewBox={`0 0 ${W} ${height}`}
      preserveAspectRatio="none"
      onMouseMove={onMove}
      onMouseLeave={() => setHover(null)}
      role="img"
      aria-label="chart"
    >
      <defs>
        <linearGradient id="areaFill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--primary)" stopOpacity="0.18" />
          <stop offset="100%" stopColor="var(--primary)" stopOpacity="0.01" />
        </linearGradient>
      </defs>

      {[0.25, 0.5, 0.75].map((f) => (
        <line
          key={f}
          x1={pad.l} x2={W - pad.r}
          y1={pad.t + innerH * f} y2={pad.t + innerH * f}
          className="grid-line"
        />
      ))}
      <line x1={pad.l} x2={W - pad.r} y1={baseline} y2={baseline} className="grid-base" />

      <path d={area} fill="url(#areaFill)" />
      <path d={line} fill="none" stroke="var(--primary)" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />

      {labels.map((l, i) =>
        l ? (
          <text key={i} x={pts[i][0]} y={height - 8} textAnchor="middle" className="axis-label">{l}</text>
        ) : null
      )}

      {h != null && (
        <g>
          <line x1={pts[h][0]} x2={pts[h][0]} y1={pad.t} y2={baseline} className="hover-line" />
          <circle cx={pts[h][0]} cy={pts[h][1]} r="4" fill="var(--surface)" stroke="var(--primary)" strokeWidth="2" />
          <g transform={`translate(${tipX},${Math.max(pts[h][1] - 44, 2)})`}>
            <rect width={tipW} height="34" rx="6" className="tip-box" />
            <text x={tipW / 2} y="14" textAnchor="middle" className="tip-value">{format(data[h])}</text>
            <text x={tipW / 2} y="27" textAnchor="middle" className="tip-label">{labels[h] || hoverLabel(labels, h)}</text>
          </g>
        </g>
      )}
    </svg>
  );
}

function hoverLabel(labels, idx) {
  for (let i = idx; i >= 0; i--) if (labels[i]) return labels[i];
  return "";
}

/* ---------------- overview ---------------- */

function OverviewPage({ overview, navigate }) {
  const [mode, setMode] = useState("activity");
  const summary = overview.catalog_health?.summary;
  const connection = overview.connection;
  const agents = overview.agents || [];
  const products = overview.catalog_health?.products || [];
  const needAttention = products.filter((p) => (p.issues || []).length > 0);
  const orders = (overview.recent_checkouts || []).slice(0, 5);
  const fetches24h = agents.reduce((acc, a) => acc + (a.hits_24h || 0), 0);
  const activeAgents = agents.filter((a) => a.status === "active").length;

  const totalActivity = (agents[0]?.activity || []).map((_, i) =>
    agents.reduce((acc, a) => acc + ((a.activity || [])[i] || 0), 0)
  );
  const activityLabels = totalActivity.map((_, i) => {
    if (i === totalActivity.length - 1) return "now";
    const back = totalActivity.length - 1 - i;
    return back % 6 === 0 ? `${back}h ago` : "";
  });

  const trend = connection?.payments?.trend || [];
  const dayNames = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  const trendLabels = trend.map((_, i) => {
    const d = new Date();
    d.setDate(d.getDate() - (trend.length - 1 - i));
    return i === trend.length - 1 ? "Today" : dayNames[d.getDay()];
  });

  return (
    <>
      <section className="stat-row">
        <div className="card stat">
          <div className="stat-label">Products live</div>
          <div className="stat-value">{summary?.total ?? 0}</div>
          <div className="stat-context">{summary?.ready_all ?? 0} fully agent ready</div>
        </div>
        <div className="card stat">
          <div className="stat-label">Agent fetches · 24h</div>
          <div className="stat-value">{fetches24h}</div>
          <div className="stat-context">{activeAgents} of {agents.length} agents active</div>
        </div>
        <div className="card stat">
          <div className="stat-label">Paid orders</div>
          <div className="stat-value">{connection?.payments?.paid_count ?? 0}</div>
          <div className="stat-context">settled via Trustap</div>
        </div>
        <div className="card stat">
          <div className="stat-label">Revenue</div>
          <div className="stat-value stat-accent">{money(connection?.payments?.revenue_minor ?? 0, "eur")}</div>
          <div className="stat-context">all time, paid checkouts</div>
        </div>
      </section>

      <div className="card card-flush" style={{ marginTop: "0.9rem" }}>
        <div className="card-head">
          <h2>{mode === "activity" ? "Agent fetches" : "Paid revenue"}</h2>
          <div className="seg">
            <button className={mode === "activity" ? "on" : ""} onClick={() => setMode("activity")}>24h fetches</button>
            <button className={mode === "revenue" ? "on" : ""} onClick={() => setMode("revenue")}>7d revenue</button>
          </div>
        </div>
        <div className="chart-wrap">
          {mode === "activity" ? (
            <AreaChart data={totalActivity} labels={activityLabels} format={(v) => `${v} fetch${v === 1 ? "" : "es"}`} />
          ) : (
            <AreaChart data={trend} labels={trendLabels} format={(v) => money(v, "eur")} />
          )}
        </div>
      </div>

      <div className="grid-2">
        <div className="card card-flush">
          <div className="card-head">
            <h2>Agent activity</h2>
            <button className="link-btn" onClick={() => navigate("agents")}>View surfaces</button>
          </div>
          <div className="card-body">
            {agents.map((a) => {
              const m = AGENTS_META[a.key] || { short: "?", label: a.name };
              return (
                <div className="list-row" key={a.key}>
                  <StatusDot status={a.status} />
                  <span className="grow strong">{m.label}</span>
                  <Bars data={a.activity || []} height={20} barWidth={4} gap={2} />
                  <span className="dim w-72">{a.hits_24h} · {timeAgo(a.last_fetch)}</span>
                </div>
              );
            })}
          </div>
        </div>

        <div className="card card-flush">
          <div className="card-head">
            <h2>Needs attention</h2>
            {needAttention.length > 0 && (
              <button className="link-btn" onClick={() => navigate("catalog")}>Open catalog</button>
            )}
          </div>
          <div className="card-body">
            {needAttention.length === 0 ? (
              <p className="empty-inline">Every product is fully listed. Nothing to fix.</p>
            ) : (
              needAttention.slice(0, 5).map((p) => (
                <div className="list-row" key={p.id}>
                  <span className="grow strong">{p.title}</span>
                  <ReadyCount readiness={p.readiness} />
                  <span className="chip">{ISSUE_LABELS[p.issues[0]] || p.issues[0]}{p.issues.length > 1 ? ` +${p.issues.length - 1}` : ""}</span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      <div className="card card-flush" style={{ marginTop: "0.9rem" }}>
        <div className="card-head">
          <h2>Latest orders</h2>
          <button className="link-btn" onClick={() => navigate("orders")}>View all</button>
        </div>
        <div className="card-body">
          {orders.length === 0 ? (
            <p className="empty-inline">No orders yet.</p>
          ) : (
            orders.map((c) => (
              <div className="list-row" key={c.id}>
                <OrderDot status={c.status} />
                <span className="grow strong">{c.description}</span>
                <span className="dim">{timeAgo(c.created_at)}</span>
                <span className="num strong w-90">{money(c.price, c.currency)}</span>
              </div>
            ))
          )}
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
    <div className={`card card-flush ${agent.status === "active" ? "is-active" : ""}`}>
      <div className="card-head">
        <div className="agent-head">
          <span className="agent-avatar">{meta.short}</span>
          <div className="agent-names">
            <span className="agent-name">{meta.label}</span>
            <span className="agent-vendor">{meta.vendor}</span>
          </div>
        </div>
        <StatusDot status={agent.status} withLabel />
      </div>
      <div className="card-body">
        <div className="agent-chart">
          <Bars data={agent.activity || []} height={48} barWidth={9} gap={3} />
          <div className="agent-chart-meta">
            <span><strong>{agent.hits_24h}</strong> fetches · 24h</span>
            <span>last {timeAgo(agent.last_fetch)}</span>
          </div>
        </div>
        <div className="surface-list">
          {(agent.surfaces || []).map((s) => (
            <div className="surface-row" key={s.surface}>
              <span className="surface-name">{s.surface.replace(/_/g, " ")}</span>
              <span className="surface-when">{s.last_fetch ? timeAgo(s.last_fetch) : "never"}</span>
              <a className="icon-btn" href={s.url} target="_blank" rel="noreferrer" title="Open">{ICONS.open}</a>
              <CopyButton text={s.url} />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function CopyButton({ text }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      className={`icon-btn ${copied ? "copied" : ""}`}
      title={copied ? "Copied" : "Copy URL"}
      onClick={() => {
        navigator.clipboard.writeText(text).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1300);
        });
      }}
    >
      {copied ? ICONS.check : ICONS.copy}
    </button>
  );
}

/* ---------------- catalog ---------------- */

function ReadyCount({ readiness }) {
  const entries = Object.keys(AGENTS_META);
  const ready = entries.filter((k) => readiness?.[k]).length;
  return (
    <span className="ready-squares" title={entries.map((k) => `${AGENTS_META[k].label}: ${readiness?.[k] ? "ready" : "missing data"}`).join("\n")}>
      {entries.map((k) => (
        <i key={k} className={readiness?.[k] ? "sq-ok" : "sq-bad"} />
      ))}
      <span className="dim">{ready}/{entries.length}</span>
    </span>
  );
}

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
        <span style={{ whiteSpace: "nowrap" }}><strong>{pct}%</strong> agent ready</span>
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
                <th className="num">Price</th>
                <th className="num">Stock</th>
                <th>Agent readiness</th>
                <th>Needs attention</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((p) => (
                <tr key={p.id}>
                  <td className="td-title">{p.title}</td>
                  <td className="td-nowrap num">{money(p.price, p.currency)}</td>
                  <td className={`num ${p.quantity <= 0 ? "td-bad" : ""}`}>{p.quantity}</td>
                  <td className="td-nowrap"><ReadyCount readiness={p.readiness} /></td>
                  <td>
                    {(p.issues || []).length === 0 ? (
                      <span className="ok-text">{ICONS.check} Complete</span>
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
      <div className="card card-flush">
        <div className="card-head">
          <h2>Trustap payments</h2>
          <StatusDot status={connection?.trustap?.connected ? "active" : "waiting"} withLabel
            labels={{ active: "Connected", waiting: "Not connected" }} />
        </div>
        <div className="card-body">
          <p className="conn-detail">
            {connection?.trustap?.connected
              ? "Checkouts settle through your Trustap account with buyer protection on every order."
              : "Add your Trustap API credentials to start selling."}
          </p>
        </div>
        <div className="card-foot">Merchant of record stays with you; Trustap handles the payment flow.</div>
      </div>

      <div className="card card-flush">
        <div className="card-head">
          <h2>Payment notifications</h2>
          <StatusDot status={connection?.webhooks?.last_event ? "active" : "waiting"} withLabel
            labels={{ active: "Receiving", waiting: "No events yet" }} />
        </div>
        <div className="card-body">
          <p className="conn-detail">
            Trustap notifies the Index when buyers pay; orders and stock update automatically.
          </p>
        </div>
        <div className="card-foot">
          {connection?.webhooks?.last_event
            ? `Last event ${timeAgo(connection.webhooks.last_event)}`
            : "Waiting for the first webhook from Trustap"}
        </div>
      </div>

      <div className="card card-flush">
        <div className="card-head">
          <h2>Store sync</h2>
          <StatusDot status={connection?.store_sync?.status === "connected" ? "active" : "waiting"} withLabel
            labels={{ active: "Synced", waiting: "Not connected" }} />
        </div>
        <div className="card-body">
          <p className="conn-detail">{connection?.store_sync?.note}</p>
          <button className="btn btn-disabled btn-small" disabled>Connect your store (coming soon)</button>
        </div>
        <div className="card-foot">Two-way inventory sync arrives with the store connector.</div>
      </div>
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
          <tr><th>Order</th><th>Status</th><th>Trustap tx</th><th>Created</th><th className="num">Amount</th></tr>
        </thead>
        <tbody>
          {checkouts.map((c) => (
            <tr key={c.id}>
              <td className="td-title">
                {c.description}
                <span className="row-sub">{c.id.slice(0, 8)}</span>
              </td>
              <td><OrderDot status={c.status} withLabel /></td>
              <td className="num">{c.transaction_id || "-"}</td>
              <td className="td-nowrap">{timeAgo(c.created_at)}</td>
              <td className="td-nowrap num strong">{money(c.price, c.currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ---------------- shared ---------------- */

function StatusDot({ status, withLabel, labels }) {
  const meta = {
    active: { label: labels?.active || "Active", cls: "dot-green", hint: "Fetched your catalog in the last 24h" },
    quiet: { label: labels?.quiet || "Quiet", cls: "dot-amber", hint: "Has fetched before, nothing in the last 24h" },
    waiting: { label: labels?.waiting || "Ready", cls: "dot-gray", hint: "Live, waiting for first activity" },
  }[status] || { label: status, cls: "dot-gray" };
  return (
    <span className="status-dot" title={meta.hint}>
      <i className={meta.cls} />
      {withLabel && <span>{meta.label}</span>}
    </span>
  );
}

function OrderDot({ status, withLabel = false }) {
  const meta = {
    paid: { label: "Paid", cls: "dot-green" },
    pending_payment: { label: "Pending payment", cls: "dot-amber" },
    cancelled: { label: "Cancelled", cls: "dot-gray" },
    failed: { label: "Failed", cls: "dot-red" },
  }[status] || { label: status, cls: "dot-gray" };
  return (
    <span className="status-dot">
      <i className={meta.cls} />
      {withLabel ? <span>{meta.label}</span> : <span className="sr-only">{meta.label}</span>}
    </span>
  );
}
