# Trustap Index: Vision and Architecture

Recorded 2026-06-12 from Dino's brief. This is the north-star document for the project; update it when the model changes, and keep the Open Questions section honest.

## Context

The Agentic Commerce MVP (`~/Projects/Agentic commerce`) proved that one protocol-agnostic core can serve every AI shopping surface (Google UCP, OpenAI ACP, Microsoft Copilot, plus feed/page-based discovery for Perplexity). That project served its purpose: **the initiative is funded**. Trustap Index is the funded build.

## Mission

Trustap becomes **both the payment layer and the marketplace** for agentic commerce. Merchants' products are duplicated into our own catalog (the Index), we serve them to all AI agents through one unified catalog and one unified checkout, and **Trustap is the merchant of record**.

## The model

1. **Catalog ingestion (duplicate, not federate).** We pull merchants' products into the Index and store them ourselves. One unified product schema serves every agent surface. Connectors per source platform (Shopify first is the obvious candidate; the team already ships Trustap Payments there).
2. **Distribution.** The Index exposes the proven agent surfaces from the MVP: UCP / ACP / Copilot adapters, GMC-spec CSV feed, public product pages with Schema.org JSON-LD.
3. **Unified checkout + real payments.** All agent checkouts converge on one checkout core. At point of sale we call the **real Trustap REST API**, exactly as the shopify_plugin project does:
   - `POST {trustap_url}/me/transactions/create_with_guest_user` creates the transaction
   - the buyer is sent to the actions page: `{actions_page}/online/transactions/{id}/guest_pay?redirect_uri=...&state=...`
   - reference implementation: `~/Projects/shopify_plugin/app/trustap.online.server.js` and `partners.trustap.server.js`
4. **Post-purchase actions.** On a completed sale: decrement Index inventory, **push the decrement to the merchant's own site**, and run the fulfillment lifecycle (Trustap business rules: automatic delivery confirmation, 24h complaint window, then release).
5. **Two-way inventory sync.** Merchants also sell outside the Index, so they notify us on their sales (webhooks, e.g. Shopify `orders/create`) and we keep Index stock current. Sync is bidirectional: Index sale -> merchant decrement; merchant sale -> Index decrement.
6. **Shipping (parked).** Either sync shipping options from the merchant, or run our own shipping integration. Note: the Trustap API already exposes **Shippo endpoints on transactions** (`shippo_address`, `shippo_parcel_details`, `shippo_shipping_rates`, `shippo_shipping_rate`; see `trustap.online.server.js`), so "our own integration" may be mostly built already.

## Open questions (decide later, do not lose)

- **Payment UX:** iframe the Trustap actions page inside Index checkout, or build our own payment UI on top of the API? (Dino: "to be answered later.")
- **Shipping:** merchant-synced options vs own integration (Shippo via Trustap API looks promising).
- **Merchant onboarding:** as merchant of record we own KYC, returns SLAs, and dispute liability; onboarding flow and contracts are open.
- **Sync transport per platform:** Shopify webhooks are easy; what about non-Shopify merchants (polling feeds? partner APIs?).
- **Conflict handling in two-way sync:** simultaneous sales of last-unit stock; reservation windows vs oversell policy.

## What we inherit from the MVP (seeded into this repo)

- Protocol-agnostic core services: catalog, checkout state machine, orders, fulfillment, merchants, webhooks
- Protocol adapters: UCP, ACP, Copilot (ACP forward-tolerant of the queued breaking order schema)
- Discovery surface: GMC CSV feed, public product pages with JSON-LD
- Data model: 8 tables, money in integer cents, EUR default
- Known gaps carried over: stubbed Trustap client, permissive agent auth, no rate limiting, EUR-only, no tax engine

## Phases (proposal)

1. **Real payments.** Replace the stubbed `TrustapClient` with the real REST integration (pattern from shopify_plugin): create transaction at complete-checkout, return actions page URL, verify inbound webhooks, drive order status from Trustap transaction lifecycle.
2. **Ingestion + sync.** Shopify connector first: import products into the Index, subscribe to inventory/order webhooks, push decrements back. This replaces "merchants hand-type products" with "merchants connect their store".
3. **Unify and harden.** One canonical product/offer model across sources, agent auth done properly, idempotency everywhere, rate limiting, multi-currency.
4. **Shipping.** Resolve the parked question; likely Shippo-via-Trustap.
5. **Go live.** Real merchants, real agents, Trustap as merchant of record.

## Related

- Confluence: RND space, Agentic Commerce folder (Overview, Baseline, Technical Reference, News, protocol research)
- Reference code: `~/Projects/shopify_plugin` (real Trustap API usage), `~/Projects/Agentic commerce` (the funded MVP this repo was seeded from)
