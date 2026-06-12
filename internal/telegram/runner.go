package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"bottrade/internal/config"
	"bottrade/internal/orders"
	"bottrade/internal/plans"

	tgbot "github.com/go-telegram/bot"
)

type PollingRunner struct {
	bot    *tgbot.Bot
	logger *slog.Logger
}

func NewPollingRunner(cfg config.Config, orderService *orders.Service, statusService *orders.StatusService, planService *plans.Service, logger *slog.Logger) (*PollingRunner, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if orderService == nil {
		orderService = orders.NewService(cfg.App.DryRun, cfg.App.ConfirmationTTL, logger)
	}
	if statusService == nil {
		statusService = orders.NewStatusService(nil)
	}
	if planService == nil {
		planService = plans.NewService(nil)
	}
	handler := NewHandlerWithServicesAndPlans(cfg.Telegram.AdminUserID, cfg.Telegram.AllowedUserIDs, cfg.App.MaxLeverage, orderService, statusService, planService, logger)
	b, err := tgbot.New(
		cfg.Telegram.BotToken,
		tgbot.WithDefaultHandler(handler.BotHandler),
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates{"message", "callback_query"}),
		tgbot.WithErrorsHandler(func(err error) {
			logger.Error("telegram polling error", "error", err)
		}),
		tgbot.WithSkipGetMe(),
	)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	return &PollingRunner{
		bot:    b,
		logger: logger,
	}, nil
}

func (r *PollingRunner) Run(ctx context.Context) error {
	r.logger.Info("telegram polling started")
	r.bot.Start(ctx)
	r.logger.Info("telegram polling stopped")
	return nil
}
