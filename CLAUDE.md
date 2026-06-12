# Trustap Index

Unified agentic marketplace with **Trustap as both the payment layer and the merchant of record**. Merchants' catalogs are duplicated into the Index; AI agents (Google UCP, OpenAI ACP, Microsoft Copilot, Perplexity via feeds) discover and buy through one unified catalog and checkout; payments run on the real Trustap REST API; inventory syncs both ways with the merchant's own store.

Read `docs/VISION.md` first: it is the north-star spec (model, open questions, phases).

## Stack

Go (org standard), built on the **Trustap Go API template** (`~/Projects/api_template_base`, instantiated here 2026-06-12 as module `github.com/trustap/trustap_index`). Spec-first: swagger YAML drives a custom code generator that emits the typed server core. Postgres via `docker compose up -d` (port 5433, db `trustap_index`).

## Template workflow (follow it, do not fight it)

1. Define endpoints in `api/swagger/paths.yaml`, models in `api/swagger/definitions.yaml` (swagger 2.0; `x-toc-endpoint` block with `context: core` and `authorised_roles` is required per endpoint).
2. `make gen_code` regenerates `target/gen/swagger_server/` (never edit generated code).
3. Implement handler methods on `API` in `internal/svc/api.go` (the generated `core.Api` interface enforces signatures at compile time).
4. App dependencies (DB pool, Trustap client, ...) go on `middleware.Context` in `internal/middleware/ctx.go`, initialized in `cmd/api/main.go`.
5. `make api` builds to `target/artfs/api`; run with `./target/artfs/api [configs/api.yaml] :8080`.

Commands: `make api`, `make run`, `make gen_code`, `make clean`; `just` for fmt/check_style/docker recipes.

## Current state (2026-06-12)

**Catalog + agent surfaces are BUILT** (same day, after the payment flow): `Product` model in Postgres (flat, no variants in v1), product CRUD under `/api/merchants/{id}/products` and `/api/products/{id}`, product-aware checkout (`product_id` + `quantity` derive price/description and check stock; raw price/description still accepted), and the discovery surfaces: ACP manifest + JSON feed (`/api/acp/{id}/.well-known/agentic-commerce`, `/api/acp/{id}/feed`), Copilot manifest + feed, UCP manifest, GMC CSV (`/feeds/{id}/gmc.csv`), and HTML product pages with Schema.org JSON-LD (`/shop/{id}`, `/shop/{id}/products/{pid}`). CSV/HTML are served by `internal/surfaces` via a middleware step because the swagger codegen only produces JSON. First post-purchase action wired: the paid webhook decrements product inventory exactly once (guarded against duplicate paid-family events). Verified end-to-end including a real product-based p2p transaction (53633, "Wireless Headphones x2"). Trustap requires a ToS IP for guest users; checkout falls back to a placeholder when agents omit `buyer_ip` (TODO: capture requester IP). Next: ACP checkout_sessions adapter (protocol-native agent checkout), merchant-side inventory sync, ingestion connectors.

Phase 1 payment flow is BUILT and **verified end-to-end on stage** (2026-06-12, first via the basic/online flow with transaction 35067, then switched to p2p). Pieces: `internal/trustap` (v1 client), `internal/store` (gorm + Postgres), checkout/webhook endpoints in swagger + `internal/svc`. Stage credentials live in gitignored `configs/api.yaml` (per-merchant API key + consented seller sub; webhook auth pair mirrors the Trustap internal dashboard entry).

**Flow decision (Dino, 2026-06-12): use the p2p flow, not basic/online.** Trustap v2 will generalise from p2p, so p2p means minimal refactoring later. Model: deposit-based with `skip_remainder: true` (deposit = full price); endpoints `GET /p2p/charge`, `POST /p2p/me/transactions/create_with_guest_user` (body uses `deposit_price` / `deposit_charge` / `deposit_charge_seller`), reads via `GET /p2p/transactions/{id}`; buyer pays at `{actions}/f2f/transactions/{id}/pay_deposit?...`. p2p lifecycle: joined, deposit_paid, remainder_skipped, seller_handover_confirmed, funds_released (or cancelled / deposit_refunded). Post-payment confirmations (accept_deposit, confirm_handover) will run via EMAILS, not agents; not built yet. Reference: shopify_plugin `trustap.p2p.server.js` (incl. the confirm-handover polling gotcha: accept_deposit returns 200 before the remainder auto-skip commits, so confirm_handover races it and 400s with `remainder_required`).

**Verified webhook payload shape** (store raw, always): `{"code": "basic_tx.paid", "time": ..., "user_id": ..., "target_id": "35067" (STRING), "target_preview": {full transaction object incl. "status"}}`. Status mapping reads `target_preview.status` (authoritative); event-code matching is the fallback. Post-payment lifecycle states (tracked/delivered/complaint/released) are stored but intentionally unmapped until the post-purchase phase.

**Known stage facts:** payment starts a tracking-details window (deadline = paid + 4 days); transactions can be batch-cancelled externally on stage (35065/35066 died that way; cause unknown, ask internally). The webhook tunnel (cloudflared quick tunnel) is ephemeral; permanent deployment home still undecided.

## Gotchas

- **Template bug, fixed here (consider upstreaming to api_template_base):** the template's "add loggers" middleware step wrote an InternalServerError body even when the endpoint had already sent a client-error response, producing two concatenated JSON bodies. Our `cmd/api/main.go` now checks for `swagger_rest.HandleEndpointError` (whose contract says the response was already written) before writing.
- **`gofumpt` and `goimports` must be on PATH** (installed at `~/go/bin`; the Makefile invokes them after codegen with `|| true`, and without them the generated code keeps an unused self-import and the build fails with "import cycle not allowed"). If a build fails that way: `export PATH="$HOME/go/bin:$PATH"`, `rm -rf target/gen/swagger_server`, `make api`.
- Private dependency `github.com/trustap/rest_api` provides logging, middleware, and REST response helpers; it resolves from the local module cache.
- Health check is `GET /api/heartbeat` (204), defined in swagger like every other endpoint.
- Request body size is capped at 2 Kb by default in `cmd/api/main.go` (`NewMaxBytesStep`); raise per-route when ingestion endpoints arrive.

## Reference projects (read-only)

- `~/Projects/api_template_base`: the pristine template; diff against it before changing infrastructure files.
- `~/Projects/Agentic commerce`: the funded Python MVP. Frozen, leave alone. Reference for protocol adapter behaviour (UCP / ACP / Copilot wire formats, GMC feed, JSON-LD storefront pages) when reimplementing in Go.
- `~/Projects/shopify_plugin`: real Trustap REST API usage. Key files: `app/trustap.online.server.js` (`POST /me/transactions/create_with_guest_user`, actions page URL `{actions_page}/online/transactions/{id}/guest_pay?...`, Shippo shipping endpoints) and `app/partners.trustap.server.js`.

## Domain rules

- Money is integer minor units (cents); currency is 3-letter ISO, EUR default.
- Trustap business rules: automatic delivery confirmation, 24h complaint window, no manual buyer confirm.
- Customer-facing wording: never say "escrow"; say buyer protection.
- ACP order fields must be treated as open enums (the queued spec change makes unknown values mandatory to accept); ACP line-item quantity arrives as int today and as `{ordered, current, fulfilled}` in the next spec version.
- Documentation: never use em dashes; no AI attribution anywhere.

## Confluence

Research and docs publish to the RND space, Agentic Commerce folder (Tech Reference, Baseline, News, per-protocol research). Durable approval exists to create/edit/delete there without asking. Local-first: save docs in the repo before pushing.
