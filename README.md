# Resilient Settlement Orchestrator

A production-grade **Payment Orchestration + Ledger System** built in Go that handles multi-PSP routing, double-entry bookkeeping, webhook management, and automated reconciliation.

## Features

- **Multi-PSP Abstraction** - Unified interface for Stripe, Razorpay, and Mock PSPs
- **Intelligent Routing** - Rule-based payment routing with cost optimization and fallback
- **Double-Entry Ledger** - Financial-grade bookkeeping with balance integrity guarantees
- **Failure Handling** - Circuit breakers, retries with exponential backoff, fallback routing
- **Webhook Processing** - Idempotent, signature-verified webhook handling with DLQ
- **Reconciliation Engine** - Real-time and batch reconciliation with discrepancy detection
- **Complete Audit Trail** - Every state transition, ledger entry, and webhook is recorded
- **Observability** - Structured logging, Prometheus metrics, health checks

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.25+ |
| HTTP Router | go-chi/chi v5 |
| Database | PostgreSQL 16 |
| Cache/Queue | Redis 7 |
| DB Driver | jackc/pgx v5 |
| Migrations | golang-migrate |

## Quick Start

```bash
# Clone
git clone https://github.com/Slambot01/resilient-settlement-orchestrator.git
cd resilient-settlement-orchestrator

# Start dependencies
docker compose up -d

# Copy env
cp .env.example .env

# Run
go run ./cmd/server

# Health check
curl http://localhost:8080/healthz
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check |
| GET | `/readyz` | Readiness probe |
| POST | `/v1/payments` | Create payment |
| GET | `/v1/payments/{id}` | Get payment status |
| POST | `/v1/payments/{id}/capture` | Capture payment |
| POST | `/v1/payments/{id}/refund` | Refund payment |
| POST | `/v1/payments/{id}/cancel` | Cancel payment |

## Architecture

See [`docs/architecture.md`](docs/architecture.md) for the full architecture document.

## License

MIT
