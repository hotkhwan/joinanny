package parser

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
)

const defaultMaxLeverage = 20

type Options struct {
	MaxLeverage int
}

type Parser struct {
	maxLeverage int
}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

func New(options Options) Parser {
	maxLeverage := options.MaxLeverage
	if maxLeverage <= 0 {
		maxLeverage = defaultMaxLeverage
	}

	return Parser{maxLeverage: maxLeverage}
}

func Parse(text string, options Options) (domain.Intent, error) {
	return New(options).Parse(text)
}

func (p Parser) Parse(text string) (domain.Intent, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return domain.Intent{}, ValidationError{Message: "Command is empty. Use /help for the command grammar."}
	}

	tokens := strings.Fields(raw)
	if len(tokens) == 0 {
		return domain.Intent{}, ValidationError{Message: "Command is empty. Use /help for the command grammar."}
	}

	switch strings.ToLower(tokens[0]) {
	case "long", "short":
		return p.parseOpen(raw, tokens)
	case "close":
		return parseClose(raw, tokens)
	case "be":
		return parseBreakeven(raw, tokens)
	case "move":
		return parseMoveStop(raw, tokens)
	case "trail":
		return parseTrail(raw, tokens)
	case "add":
		return parseAdd(raw, tokens)
	case "status", "/status":
		return domain.Intent{Type: domain.IntentStatus, RawText: raw}, nil
	case "plan":
		return parsePlanStatus(raw, tokens)
	default:
		return domain.Intent{}, ValidationError{Message: "Command not recognized yet. Use /help for the command grammar."}
	}
}

func (p Parser) parseOpen(raw string, tokens []string) (domain.Intent, error) {
	if len(tokens) != 11 && len(tokens) != 13 {
		return domain.Intent{}, ValidationError{Message: "Open order format: long BTC 3x entry 67500 sl 65000 tp 72000 size 100usdt"}
	}

	side := domain.Side(strings.ToLower(tokens[0]))
	symbol, err := normalizeSymbol(tokens[1])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	leverage, err := p.parseLeverage(tokens[2])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	if strings.ToLower(tokens[3]) != "entry" {
		return domain.Intent{}, ValidationError{Message: "Open order must include entry <price>."}
	}
	entry, err := parsePositiveDecimal("entry", tokens[4])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	if strings.ToLower(tokens[5]) != "sl" {
		return domain.Intent{}, ValidationError{Message: "Open order must include sl <price>."}
	}
	stopLoss, err := parsePositiveDecimal("sl", tokens[6])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	if strings.ToLower(tokens[7]) != "tp" {
		return domain.Intent{}, ValidationError{Message: "Open order must include exactly one tp <price> in Phase 1."}
	}
	takeProfit, err := parsePositiveDecimal("tp", tokens[8])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	size, err := parseSize(tokens[9], tokens[10])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	planID := ""
	if len(tokens) == 13 {
		if strings.ToLower(tokens[11]) != "plan" {
			return domain.Intent{}, ValidationError{Message: "Optional plan tag format: plan <1|2|3>."}
		}
		planID, err = parsePlanID(tokens[12])
		if err != nil {
			return domain.Intent{}, ValidationError{Message: err.Error()}
		}
	}

	if err := validateStopAndTakeProfit(side, entry, stopLoss, takeProfit); err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	return domain.Intent{
		Type:    domain.IntentOpen,
		RawText: raw,
		Open: &domain.OpenIntent{
			Symbol:      symbol,
			Side:        side,
			Leverage:    leverage,
			Entry:       entry,
			StopLoss:    stopLoss,
			TakeProfits: []decimal.Decimal{takeProfit},
			Size:        size,
			PlanID:      planID,
		},
	}, nil
}

func parseClose(raw string, tokens []string) (domain.Intent, error) {
	if len(tokens) < 2 || len(tokens) > 3 {
		return domain.Intent{}, ValidationError{Message: "Close format: close BTC, close BTC 50%, or close all."}
	}

	if strings.EqualFold(tokens[1], "all") {
		if len(tokens) != 2 {
			return domain.Intent{}, ValidationError{Message: "close all cannot include a percentage."}
		}
		return domain.Intent{
			Type:    domain.IntentClose,
			RawText: raw,
			Close: &domain.CloseIntent{
				All:             true,
				ResolvedPercent: decimal.NewFromInt(100),
			},
		}, nil
	}

	symbol, err := normalizeSymbol(tokens[1])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	closeIntent := &domain.CloseIntent{
		Symbol:          symbol,
		ResolvedPercent: decimal.NewFromInt(100),
	}
	if len(tokens) == 3 {
		percent, err := parseClosePercent(tokens[2])
		if err != nil {
			return domain.Intent{}, ValidationError{Message: err.Error()}
		}
		closeIntent.Percent = percent
		closeIntent.HasPercent = true
		closeIntent.ResolvedPercent = percent
	}

	return domain.Intent{
		Type:    domain.IntentClose,
		RawText: raw,
		Close:   closeIntent,
	}, nil
}

func parseBreakeven(raw string, tokens []string) (domain.Intent, error) {
	if len(tokens) != 2 {
		return domain.Intent{}, ValidationError{Message: "Breakeven format: be BTC."}
	}

	symbol, err := normalizeSymbol(tokens[1])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	return domain.Intent{
		Type:    domain.IntentBreakeven,
		RawText: raw,
		Breakeven: &domain.BreakevenIntent{
			Symbol: symbol,
		},
	}, nil
}

func parseMoveStop(raw string, tokens []string) (domain.Intent, error) {
	if len(tokens) != 5 || !strings.EqualFold(tokens[1], "sl") || !strings.EqualFold(tokens[3], "to") || !strings.EqualFold(tokens[4], "be") {
		return domain.Intent{}, ValidationError{Message: "Move stop format: move sl BTC to be."}
	}

	symbol, err := normalizeSymbol(tokens[2])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	return domain.Intent{
		Type:    domain.IntentBreakeven,
		RawText: raw,
		Breakeven: &domain.BreakevenIntent{
			Symbol: symbol,
		},
	}, nil
}

func parseTrail(raw string, tokens []string) (domain.Intent, error) {
	if len(tokens) != 3 {
		return domain.Intent{}, ValidationError{Message: "Trailing stop format: trail BTC 0.5%."}
	}

	symbol, err := normalizeSymbol(tokens[1])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}
	callbackRate, err := parseTrailPercent(tokens[2])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	return domain.Intent{
		Type:    domain.IntentTrail,
		RawText: raw,
		Trail: &domain.TrailIntent{
			Symbol:       symbol,
			CallbackRate: callbackRate,
		},
	}, nil
}

func parseAdd(raw string, tokens []string) (domain.Intent, error) {
	if len(tokens) != 4 {
		return domain.Intent{}, ValidationError{Message: "Scale-in format: add BTC size 100usdt or add BTC qty 0.01."}
	}

	symbol, err := normalizeSymbol(tokens[1])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}
	size, err := parseSize(tokens[2], tokens[3])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	return domain.Intent{
		Type:    domain.IntentAdd,
		RawText: raw,
		Add: &domain.AddIntent{
			Symbol: symbol,
			Size:   size,
		},
	}, nil
}

func parsePlanStatus(raw string, tokens []string) (domain.Intent, error) {
	if len(tokens) != 3 || strings.ToLower(tokens[2]) != "status" {
		return domain.Intent{}, ValidationError{Message: "Plan status format: plan <1|2|3> status."}
	}

	planID, err := parsePlanID(tokens[1])
	if err != nil {
		return domain.Intent{}, ValidationError{Message: err.Error()}
	}

	return domain.Intent{
		Type:    domain.IntentPlanStatus,
		RawText: raw,
		PlanStatus: &domain.PlanStatusIntent{
			PlanID: planID,
		},
	}, nil
}

func (p Parser) parseLeverage(token string) (int, error) {
	value := strings.ToLower(strings.TrimSpace(token))
	if !strings.HasSuffix(value, "x") {
		return 0, fmt.Errorf("Leverage must use <int>x, for example 3x.")
	}

	leverage, err := strconv.Atoi(strings.TrimSuffix(value, "x"))
	if err != nil || leverage <= 0 {
		return 0, fmt.Errorf("Leverage must be a positive integer.")
	}
	if leverage > p.maxLeverage {
		return 0, fmt.Errorf("Leverage %dx exceeds MAX_LEVERAGE %dx.", leverage, p.maxLeverage)
	}

	return leverage, nil
}

func parseSize(kindToken string, amountToken string) (domain.OrderSize, error) {
	switch strings.ToLower(kindToken) {
	case "size":
		amount := strings.ToLower(strings.TrimSpace(amountToken))
		if !strings.HasSuffix(amount, "usdt") {
			return domain.OrderSize{}, fmt.Errorf("size must use <amount>usdt, for example size 100usdt.")
		}
		value, err := parsePositiveDecimal("size", strings.TrimSuffix(amount, "usdt"))
		if err != nil {
			return domain.OrderSize{}, err
		}
		return domain.OrderSize{Kind: domain.SizeUSDT, Amount: value}, nil
	case "qty":
		value, err := parsePositiveDecimal("qty", amountToken)
		if err != nil {
			return domain.OrderSize{}, err
		}
		return domain.OrderSize{Kind: domain.SizeQty, Amount: value}, nil
	default:
		return domain.OrderSize{}, fmt.Errorf("Open order must include size <amount>usdt or qty <amount>.")
	}
}

func parsePositiveDecimal(label string, token string) (decimal.Decimal, error) {
	value, err := decimal.Parse(token)
	if err != nil {
		return decimal.Zero(), fmt.Errorf("%s must be a decimal number.", label)
	}
	if !value.IsPositive() {
		return decimal.Zero(), fmt.Errorf("%s must be greater than zero.", label)
	}

	return value, nil
}

func parseClosePercent(token string) (decimal.Decimal, error) {
	value := strings.TrimSpace(token)
	if !strings.HasSuffix(value, "%") {
		return decimal.Zero(), fmt.Errorf("Close percentage must use <percent>%%, for example close BTC 50%%.")
	}

	percent, err := parsePositiveDecimal("close percentage", strings.TrimSuffix(value, "%"))
	if err != nil {
		return decimal.Zero(), err
	}
	if percent.Cmp(decimal.NewFromInt(100)) > 0 {
		return decimal.Zero(), fmt.Errorf("Close percentage must be at most 100%%.")
	}

	return percent, nil
}

func parseTrailPercent(token string) (decimal.Decimal, error) {
	value := strings.TrimSpace(token)
	if !strings.HasSuffix(value, "%") {
		return decimal.Zero(), fmt.Errorf("Trailing stop callback rate must use <percent>%%, for example trail BTC 0.5%%.")
	}

	percent, err := parsePositiveDecimal("trailing stop callback rate", strings.TrimSuffix(value, "%"))
	if err != nil {
		return decimal.Zero(), err
	}
	if percent.Cmp(decimal.NewFromInt(10)) > 0 {
		return decimal.Zero(), fmt.Errorf("Trailing stop callback rate must be at most 10%%.")
	}

	return percent, nil
}

func parsePlanID(token string) (string, error) {
	switch strings.TrimSpace(token) {
	case "1", "2", "3":
		return strings.TrimSpace(token), nil
	default:
		return "", fmt.Errorf("Plan must be 1, 2, or 3.")
	}
}

func normalizeSymbol(token string) (string, error) {
	symbol := strings.ToUpper(strings.TrimSpace(token))
	symbol = strings.ReplaceAll(symbol, "/", "")
	if symbol == "" {
		return "", fmt.Errorf("Symbol is required.")
	}
	for _, r := range symbol {
		if !unicode.IsDigit(r) && (r < 'A' || r > 'Z') {
			return "", fmt.Errorf("Symbol may contain only letters and digits.")
		}
	}

	if !strings.HasSuffix(symbol, "USDT") {
		symbol += "USDT"
	}
	if strings.TrimSuffix(symbol, "USDT") == "" {
		return "", fmt.Errorf("Symbol must include a base asset, for example BTC.")
	}

	return symbol, nil
}

func validateStopAndTakeProfit(side domain.Side, entry decimal.Decimal, stopLoss decimal.Decimal, takeProfit decimal.Decimal) error {
	switch side {
	case domain.SideLong:
		if stopLoss.Cmp(entry) >= 0 {
			return fmt.Errorf("Long stop loss must be below entry.")
		}
		if takeProfit.Cmp(entry) <= 0 {
			return fmt.Errorf("Long take profit must be above entry.")
		}
	case domain.SideShort:
		if stopLoss.Cmp(entry) <= 0 {
			return fmt.Errorf("Short stop loss must be above entry.")
		}
		if takeProfit.Cmp(entry) >= 0 {
			return fmt.Errorf("Short take profit must be below entry.")
		}
	default:
		return fmt.Errorf("Side must be long or short.")
	}

	return nil
}
