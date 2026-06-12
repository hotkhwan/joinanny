---
name: go-trade-code-style
description: Use when writing or reviewing Go code for the trading bot so package structure, error handling, tests, logging, and external boundaries stay consistent.
---

# Go Trade Code Style

Use this skill for Go implementation and review.

## Package Layout

Recommended package responsibilities:

- `cmd/tradebot`: process entrypoint only
- `internal/app`: dependency wiring and lifecycle
- `internal/config`: environment loading and validation
- `internal/domain`: core business types and enums
- `internal/parser`: free-form text parsing
- `internal/telegram`: Telegram commands, messages, callbacks
- `internal/orders`: order intent, risk checks, confirmation flow
- `internal/exchange/binance`: Binance Futures adapter
- `internal/storage/mongo`: MongoDB repositories
- `internal/storage/object`: S3-compatible object storage
- `internal/api`: optional Fiber v3 routes
- `internal/monitor`: background alert/position loops

## Code Rules

- Keep handlers thin and services testable.
- Pass `context.Context` into IO and background work.
- Put interfaces at external boundaries, not everywhere.
- Wrap errors with operation context.
- Keep package names short and lowercase.
- Avoid hidden goroutine leaks. Background loops need cancellation.
- Avoid global mutable state outside process wiring.
- Validate config at startup and fail fast.
- Keep logging structured and never log secrets.

## Trading Math

- Do not use `float64` for order-critical math.
- Use a decimal type for prices, quantities, balances, risk, and P&L.
- Normalize symbols and sides at the domain boundary.
- Apply exchange filters before order submission.
- Phase 1 open intents must include explicit `size <amount>usdt` or `qty <amount>`.
- Default margin mode is isolated.
- Model order sizing explicitly as `OrderSize{Kind, Amount}` with separate size kinds for USDT notional and base quantity.
- Use atomic conditional updates for confirmation status transitions.

## Tests

Prefer table-driven tests for:

- Parser grammar
- Risk validation
- Quantity/price rounding
- Confirmation idempotency
- Confirmation TTL persistence
- `close <symbol>` defaults to 100%
- Telegram callback routing

Use fakes for:

- Exchange client
- MongoDB repositories
- Object storage
- Telegram sender
- Clock/time source

Network integration tests must be opt-in and clearly marked.

## Fiber v3

Use Fiber v3 only for HTTP needs:

- `GET /healthz`
- `GET /readyz`
- `POST /telegram/webhook`
- Admin/QA routes when explicitly useful

Routes should delegate to app services and should not contain trading logic.
