package campaign

import "bottrade/internal/decimal"

// TradeOutcome is one simulated trade in a campaign preview.
type TradeOutcome struct {
	Index       int
	Win         bool
	PnL         decimal.Decimal
	RunningPnL  decimal.Decimal
	TradesSoFar int
}

// SimulationResult is the outcome of a campaign preview.
type SimulationResult struct {
	Goal     Goal
	Outcomes []TradeOutcome
	State    State
	Verdict  Verdict
}

// Simulate runs the campaign loop with modeled outcomes instead of real orders,
// so the goal, the trade-count math, and the stop rules can be previewed with
// zero risk. Wins and losses are distributed deterministically to match the
// goal's assumed win-rate (a Bresenham-style even spread, not random), each
// booking the goal's fixed reward or risk. It stops on the same rules as a live
// run: target reached, max drawdown, or max trades.
//
// This is a preview, not execution — it never touches the exchange. Live,
// AI-driven campaigns are a separate, gated path.
func Simulate(goal Goal) SimulationResult {
	state := State{Goal: goal}
	result := SimulationResult{Goal: goal}

	// A hard ceiling independent of the goal's own MaxTrades, so a goal that can
	// never stop (e.g. MaxTrades=0 and an unreachable target) still terminates.
	const hardCap = 1000

	for i := 0; i < hardCap; i++ {
		if verdict := Evaluate(state); verdict != Continue {
			result.State = state
			result.Verdict = verdict
			return result
		}

		win := winsByNow(i+1, goal.AssumedWinRate) > winsByNow(i, goal.AssumedWinRate)
		pnl := goal.RewardPerTradeUSDT
		if !win {
			pnl = goal.RiskPerTradeUSDT.Neg()
		}
		state.RealizedPnL = state.RealizedPnL.Add(pnl)
		state.TradesClosed++
		result.Outcomes = append(result.Outcomes, TradeOutcome{
			Index:       i + 1,
			Win:         win,
			PnL:         pnl,
			RunningPnL:  state.RealizedPnL,
			TradesSoFar: state.TradesClosed,
		})
	}

	result.State = state
	result.Verdict = Evaluate(state)
	return result
}

// winsByNow returns how many of the first n trades are wins for the given
// win-rate percent, spread evenly (floor(n*rate/100)).
func winsByNow(n, winRatePercent int) int {
	if winRatePercent <= 0 {
		return 0
	}
	if winRatePercent >= 100 {
		return n
	}
	return n * winRatePercent / 100
}
