package signals

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bottrade/internal/orders"
)

type Advisor interface {
	Decide(ctx context.Context, signal MarketSignal) (Decision, error)
}

type Processor struct {
	advisor              Advisor
	orderService         *orders.Service
	signalStore          SignalStore
	adminUserID          int64
	maxLeverage          int
	minConfidencePercent int
	autoTradeEnabled     bool
	logger               *slog.Logger
}

type ProcessorConfig struct {
	Advisor              Advisor
	OrderService         *orders.Service
	SignalStore          SignalStore
	AdminUserID          int64
	MaxLeverage          int
	MinConfidencePercent int
	AutoTradeEnabled     bool
	Logger               *slog.Logger
}

type ProcessResult struct {
	Signal             MarketSignal            `json:"signal"`
	Decision           Decision                `json:"decision"`
	Accepted           bool                    `json:"accepted"`
	Executed           bool                    `json:"executed"`
	Execution          *orders.ExecutionResult `json:"execution,omitempty"`
	ConfirmationID     string                  `json:"confirmation_id,omitempty"`
	ConfirmationNeeded bool                    `json:"confirmation_needed"`
	Message            string                  `json:"message"`
}

func NewProcessor(cfg ProcessorConfig) *Processor {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxLeverage := cfg.MaxLeverage
	if maxLeverage <= 0 {
		maxLeverage = 20
	}
	minConfidence := cfg.MinConfidencePercent
	if minConfidence < 0 {
		minConfidence = 0
	}
	if minConfidence > 100 {
		minConfidence = 100
	}
	signalStore := cfg.SignalStore
	if signalStore == nil {
		signalStore = NoopSignalStore{}
	}

	return &Processor{
		advisor:              cfg.Advisor,
		orderService:         cfg.OrderService,
		signalStore:          signalStore,
		adminUserID:          cfg.AdminUserID,
		maxLeverage:          maxLeverage,
		minConfidencePercent: minConfidence,
		autoTradeEnabled:     cfg.AutoTradeEnabled,
		logger:               logger,
	}
}

func (p *Processor) Process(ctx context.Context, signal MarketSignal) (ProcessResult, error) {
	signal = signal.Sanitized()
	if err := signal.Validate(); err != nil {
		return ProcessResult{}, err
	}

	if p.advisor == nil {
		result := ProcessResult{
			Signal:   signal,
			Accepted: true,
			Decision: Decision{
				Action: ActionHold,
				Symbol: signal.Symbol,
				Reason: "AI advisor is disabled.",
			},
			Message: "Signal accepted. AI advisor is disabled.",
		}
		p.recordSignal(ctx, result, SignalStatusHeld)
		return result, nil
	}

	decision, err := p.advisor.Decide(ctx, signal)
	if err != nil {
		p.recordSignal(ctx, ProcessResult{
			Signal:  signal,
			Message: err.Error(),
		}, SignalStatusFailed)
		return ProcessResult{}, err
	}
	decision.Action = DecisionAction(stringsLowerTrim(string(decision.Action)))
	decision.Symbol = normalizeSymbol(decision.Symbol)
	if decision.Symbol == "" {
		decision.Symbol = signal.Symbol
	}

	result := ProcessResult{
		Signal:   signal,
		Decision: decision,
		Accepted: true,
	}

	if decision.Action == ActionHold {
		result.Message = "Signal accepted. AI decision is hold."
		p.recordSignal(ctx, result, SignalStatusHeld)
		return result, nil
	}

	if decision.ConfidencePercent < p.minConfidencePercent {
		result.Message = fmt.Sprintf("Signal accepted. AI confidence %d%% is below minimum %d%%.", decision.ConfidencePercent, p.minConfidencePercent)
		p.recordSignal(ctx, result, SignalStatusRejected)
		return result, nil
	}

	intent, err := DecisionToIntent(decision, p.maxLeverage)
	if err != nil {
		result.Message = err.Error()
		p.recordSignal(ctx, result, SignalStatusFailed)
		return ProcessResult{}, err
	}

	if p.orderService == nil {
		result.Message = "Signal accepted. Order service is not connected."
		p.recordSignal(ctx, result, SignalStatusAccepted)
		return result, nil
	}

	confirmation, err := p.orderService.Prepare(ctx, p.adminUserID, intent)
	if err != nil {
		result.Message = err.Error()
		p.recordSignal(ctx, result, SignalStatusFailed)
		return ProcessResult{}, err
	}
	result.ConfirmationID = confirmation.ID

	if !p.autoTradeEnabled {
		result.ConfirmationNeeded = true
		result.Message = "Signal accepted. Confirmation is required before execution."
		p.recordSignal(ctx, result, SignalStatusConfirmationRequired)
		return result, nil
	}

	execution, err := p.orderService.Confirm(ctx, p.adminUserID, confirmation.ID)
	if err != nil {
		result.Message = err.Error()
		p.recordSignal(ctx, result, SignalStatusFailed)
		return ProcessResult{}, err
	}
	result.Executed = true
	result.Execution = &execution
	result.Message = "Signal accepted and executed by autonomous testnet/dry-run flow."
	p.recordSignal(ctx, result, SignalStatusExecuted)
	p.logger.Info("ai signal executed", "symbol", signal.Symbol, "action", decision.Action, "mode", execution.Mode)

	return result, nil
}

func (p *Processor) recordSignal(ctx context.Context, result ProcessResult, status SignalStatus) {
	record := SignalRecord{
		Signal:         result.Signal,
		Decision:       result.Decision,
		Status:         status,
		Message:        result.Message,
		ConfirmationID: result.ConfirmationID,
		CreatedAt:      result.Signal.ReceivedAt,
		UpdatedAt:      result.Signal.ReceivedAt,
	}
	if result.Execution != nil {
		record.ExecutionMode = result.Execution.Mode
		record.ClientOrderID = result.Execution.ClientOrderID
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = timeNow()
		record.UpdatedAt = record.CreatedAt
	}

	if err := p.signalStore.PutSignalRecord(ctx, record); err != nil {
		p.logger.Warn("signal record failed", "symbol", result.Signal.Symbol, "status", status, "error", err)
	}
}

var timeNow = func() time.Time {
	return time.Now()
}

func stringsLowerTrim(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
