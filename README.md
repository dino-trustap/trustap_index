# Trustap Index

Unified agentic marketplace: merchants connect their store once, their products become discoverable and buyable by every AI shopping agent (Google, ChatGPT, Copilot, Perplexity), and Trustap handles payment, buyer protection, and the fulfillment lifecycle as merchant of record.

See `docs/VISION.md` for the full model, open questions, and roadmap. Built on the Trustap Go API template (swagger-based code generation, middleware infrastructure, graceful shutdown).

## Prerequisites

- Go 1.24.3 or later
- Make, (optional) just
- Docker (Postgres + containerized deployment)

## Building and running

```bash
docker compose up -d        # Postgres on port 5433, db: trustap_index

make api                    # generate code + build -> target/artfs/api
make run                    # run without config on :8080
make run-with-config        # run with configs/api.yaml on :8080

just                        # list dev recipes (build, run, fmt, check_style, build_img, run_cont)
```

Health check: `curl -v http://localhost:8080/api/heartbeat` (expect 204).

## Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/heartbeat` | Health check (204) |
| POST | `/api/checkouts` | Create checkout: Trustap guest buyer + charge + transaction; returns the actions page `pay_url` |
| GET | `/api/checkouts/{id}` | Checkout with current payment status |
| GET | `/api/checkouts/{id}/return` | Buyer redirect target after paying on the actions page |
| POST | `/api/webhooks/trustap` | Trustap webhook receiver (basic auth; same pair configured in the Trustap dashboard) |

The payment flow is the Trustap v1 full-seller/guest-buyer model: each merchant has its own Trustap API client (key + consented seller sub) configured in `configs/api.yaml`. Payment truth comes from webhooks; the buyer redirect only records that the buyer came back.

## Adding an endpoint (template workflow)

1. Define it in `api/swagger/paths.yaml` (plus models in `definitions.yaml`).
2. `make gen_code` to regenerate the typed server core into `target/gen/swagger_server/`.
3. Implement the handler method on `API` in `internal/svc/api.go`.
4. `make api` and test.

Application dependencies (DB pool, Trustap client, ...) are added to `middleware.Context` in `internal/middleware/ctx.go` and initialized in `cmd/api/main.go`.

## Configuration

Copy `configs/api.sample.yaml` to `configs/api.yaml` and edit. Logging (level, format) and server timeouts are supported; extend `cmd/api/config.go` as the app grows.
