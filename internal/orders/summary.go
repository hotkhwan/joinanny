package orders

import (
	"fmt"
	"strings"

	"bottrade/internal/domain"
)

func Summary(intent domain.Intent) string {
	switch intent.Type {
	case domain.IntentOpen:
		return openSummary(intent.Open)
	case domain.IntentClose:
		return closeSummary(intent.Close)
	case domain.IntentBreakeven:
		return breakevenSummary(intent.Breakeven)
	case domain.IntentTrail:
		return trailSummary(intent.Trail)
	case domain.IntentAdd:
		return addSummary(intent.Add)
	default:
		return "Unsupported intent."
	}
}

func openSummary(intent *domain.OpenIntent) string {
	if intent == nil || len(intent.TakeProfits) == 0 {
		return "Invalid open intent."
	}

	size := ""
	switch intent.Size.Kind {
	case domain.SizeUSDT:
		size = intent.Size.Amount.String() + " USDT"
	case domain.SizeQty:
		size = intent.Size.Amount.String() + " qty"
	default:
		size = intent.Size.Amount.String()
	}

	plan := ""
	if intent.PlanID != "" {
		plan = "\nPlan: " + intent.PlanID
	}

	return fmt.Sprintf(
		"%s %s %dx\nEntry: %s\nSL: %s\nTP: %s\nSize: %s%s",
		strings.ToUpper(string(intent.Side)),
		intent.Symbol,
		intent.Leverage,
		intent.Entry.String(),
		intent.StopLoss.String(),
		intent.TakeProfits[0].String(),
		size,
		plan,
	)
}

func closeSummary(intent *domain.CloseIntent) string {
	if intent == nil {
		return "Invalid close intent."
	}

	if intent.All {
		return "CLOSE ALL\nPercent: 100%"
	}

	return fmt.Sprintf("CLOSE %s\nPercent: %s%%", intent.Symbol, intent.ResolvedPercent.String())
}

func breakevenSummary(intent *domain.BreakevenIntent) string {
	if intent == nil {
		return "Invalid breakeven intent."
	}
	return fmt.Sprintf("MOVE SL TO BREAKEVEN\nSymbol: %s", intent.Symbol)
}

func trailSummary(intent *domain.TrailIntent) string {
	if intent == nil {
		return "Invalid trailing-stop intent."
	}
	return fmt.Sprintf("TRAIL %s\nCallback: %s%%", intent.Symbol, intent.CallbackRate.String())
}

func addSummary(intent *domain.AddIntent) string {
	if intent == nil {
		return "Invalid add intent."
	}

	size := ""
	switch intent.Size.Kind {
	case domain.SizeUSDT:
		size = intent.Size.Amount.String() + " USDT"
	case domain.SizeQty:
		size = intent.Size.Amount.String() + " qty"
	default:
		size = intent.Size.Amount.String()
	}

	return fmt.Sprintf("ADD %s\nSize: %s", intent.Symbol, size)
}
