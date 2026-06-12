# AGENTS.md

Codex-compatible entrypoint. The canonical instructions are in `AGENT.md`.

Core rules:

- Use Go for application code.
- Use Fiber v3 only when HTTP endpoints are actually needed.
- Use MongoDB Atlas for durable app data.
- Use S3-compatible storage only for generated/uploaded files.
- Keep Binance real trading disabled by default.
- Preserve user changes and do not revert unrelated work.
- Run tests when code exists and report any tests that could not be run.

Read `AGENT.md`, `CLAUDE.md`, `trading_bot_plan.md`, and `TRADING_BOT_REVIEW.md` before substantial work.
