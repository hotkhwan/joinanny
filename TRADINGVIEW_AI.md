# TradingView And AI Advisor

This bot uses TradingView as a signal source through webhook alerts. It does not use TradingView as a market-data REST API.

TradingView alert target:

```text
POST /tradingview/webhook
```

Example alert body:

```json
{
  "secret": "change-me",
  "source": "tradingview",
  "symbol": "BTCUSDT",
  "interval": "15m",
  "price": "67500",
  "strategy": "rsi_macd_volume",
  "action_hint": "evaluate",
  "side_hint": "long",
  "message": "RSI oversold with MACD cross",
  "indicators": {
    "rsi14": 28.4,
    "macd_hist": 12.5,
    "volume_change_percent": 180
  }
}
```

Common TradingView placeholders also work when mapped like this:

```json
{
  "secret": "change-me",
  "ticker": "{{ticker}}",
  "timeframe": "{{interval}}",
  "close": "{{close}}",
  "strategy": "my_strategy",
  "action": "{{strategy.order.action}}",
  "message": "{{strategy.order.comment}}"
}
```

Required local config for webhook intake:

```env
HTTP_ENABLED=true
HTTP_ADDR=:8080
TRADINGVIEW_ENABLED=true
TRADINGVIEW_WEBHOOK_SECRET=change-me
```

AI advisor is disabled by default. To use an OpenAI-compatible chat completions API:

```env
AI_ENABLED=true
AI_PROVIDER=openai_compatible
AI_API_KEY=
AI_BASE_URL=https://api.openai.com/v1
AI_MODEL=
AI_MIN_CONFIDENCE_PERCENT=70
AI_AUTOTRADE_ENABLED=false
```

Safety gates:

- `AI_AUTOTRADE_ENABLED=false` means webhook signals can produce decisions but will not execute automatically.
- If `AI_AUTOTRADE_ENABLED=true`, the bot can auto-confirm decisions only through the existing order service.
- Real Binance live execution is still not wired in this phase. Use dry-run or Binance testnet.
- TradingView webhook secrets must not be placed in public Pine scripts or shared screenshots.
