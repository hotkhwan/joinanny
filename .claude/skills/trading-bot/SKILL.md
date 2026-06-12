---
name: trading-bot
description: Use when designing, implementing, or reviewing the Go Telegram Binance Futures trading bot, including parser behavior, exchange safety, persistence, plans, scanner, alerts, and storage choices.
---

# Trading Bot Skill

Use this skill for all trading-bot domain work in this repo.

## Product Shape

The bot is a private Telegram control surface for Binance Futures. The user sends free-form trade text, the bot parses it into a typed intent, shows a confirmation, then executes only when safety gates allow it.

Primary stack:

- Go application code
- Optional Fiber v3 HTTP layer
- MongoDB Atlas for durable state
- S3-compatible storage for generated files
- Binance Futures behind an exchange interface

## Safety First

Always check these before approving or implementing exchange-changing behavior:

- `DRY_RUN=true` by default
- `BINANCE_TESTNET=true` by default
- `REAL_TRADING_ENABLED=false` by default
- Explicit user confirmation before every exchange-changing action
- Telegram allowlist enforced for messages and callbacks
- Idempotency key for each confirmation/callback
- Atomic conditional update for confirmation status transitions
- Audit event for every decision and exchange call
- Isolated margin mode by default

## Domain Model Guidance

Prefer explicit domain types:

- `OrderIntent`
- `OrderSide`
- `Position`
- `Plan`
- `Signal`
- `RiskCheck`
- `Confirmation`
- `AuditEvent`

Avoid loose `map[string]any` after parsing. It is acceptable at JSON/Mongo boundaries, but convert to typed structs quickly.

Use exact decimal math for prices, quantities, risk, and P&L. Avoid `float64` for order-critical calculations.

## Parser Requirements

Parser changes need table-driven tests. Cover at least:

- Long and short orders
- Entry, SL, TP, and leverage
- Explicit size using `size <amount>usdt` or `qty <amount>`
- Missing required fields
- Missing size/qty
- Invalid leverage
- Multiple take profits are rejected in Phase 1
- Close all
- Close symbol defaults to 100%
- Close percentage
- `/status` and free-form `status` are equivalent read-only intents
- Plan tags
- Ambiguous text

The parser should produce an intent plus validation errors. It should not call Telegram, Binance, MongoDB, or S3.

## Exchange Adapter Requirements

Exchange logic must sit behind an interface. Tests should use fakes.

Before placing an order:

- Load symbol filters
- Round price to tick size
- Round quantity to step size
- Validate minimum notional
- Validate leverage and margin mode
- Reject execution if symbol filters are unavailable
- Check dry-run/testnet/live flags
- Attach idempotency metadata where possible

## Storage Requirements

MongoDB Atlas stores durable app state:

- Audit events
- Order intents
- Confirmations
- Orders and fills
- Positions
- Plans
- Signals
- Alert/job state

Confirmation state belongs in MongoDB with a TTL index, and status changes must be compare-and-swap style updates so duplicate callbacks or multiple workers cannot execute the same intent twice.

S3-compatible storage stores generated files:

- Reports
- Audit exports
- Backtest outputs
- Chart screenshots
- Large CSV/JSON artifacts

Do not add a local database unless the user explicitly requests a local-first architecture.

## Done Criteria

Work is not done until:

- Tests are added or updated for changed behavior
- Real trading remains disabled by default
- Secrets are not committed
- New env vars are documented
- Claude review can identify how the change was tested
