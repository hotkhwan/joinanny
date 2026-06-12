package app

import (
	"context"
	"log/slog"
	"time"

	"bottrade/internal/ai"
	"bottrade/internal/api"
	"bottrade/internal/config"
	binanceexec "bottrade/internal/exchange/binance"
	"bottrade/internal/orders"
	"bottrade/internal/plans"
	"bottrade/internal/signals"
	mongostore "bottrade/internal/storage/mongo"
	"bottrade/internal/telegram"
)

type App struct {
	cfg    config.Config
	logger *slog.Logger
}

func New(cfg config.Config, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}

	return &App{
		cfg:    cfg,
		logger: logger,
	}
}

func (a *App) Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	a.logger.Info(
		"application bootstrap complete",
		"env", a.cfg.App.Env,
		"dry_run", a.cfg.App.DryRun,
		"real_trading_enabled", a.cfg.App.RealTradingEnabled,
		"telegram_mode", a.cfg.Telegram.Mode,
		"binance_testnet", a.cfg.Binance.Testnet,
		"mongodb_database", a.cfg.MongoDB.Database,
	)

	orderService, statusService, planService, signalStore, cleanup, err := a.newTradingServices(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	signalProcessor := a.newSignalProcessor(orderService, signalStore)

	errCh := make(chan error, 2)
	if a.cfg.HTTP.Enabled {
		server := api.NewServer(a.cfg, signalProcessor, a.logger)
		go func() {
			if err := server.Run(ctx); err != nil {
				errCh <- err
			}
		}()
	}

	if a.cfg.Telegram.Mode != config.TelegramModePolling {
		a.logger.Info("telegram mode is not polling; telegram polling runtime not started", "telegram_mode", a.cfg.Telegram.Mode)
		if a.cfg.HTTP.Enabled {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err := <-errCh:
				return err
			}
		}
		return nil
	}

	runner, err := telegram.NewPollingRunner(a.cfg, orderService, statusService, planService, a.logger)
	if err != nil {
		return err
	}

	if a.cfg.HTTP.Enabled {
		go func() {
			if err := runner.Run(ctx); err != nil {
				errCh <- err
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		}
	}

	return runner.Run(ctx)
}

func (a *App) newTradingServices(ctx context.Context) (*orders.Service, *orders.StatusService, *plans.Service, signals.SignalStore, func(), error) {
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	store, err := mongostore.Connect(connectCtx, mongostore.Config{
		URI:      a.cfg.MongoDB.URI,
		Database: a.cfg.MongoDB.Database,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	cleanup := func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.Disconnect(disconnectCtx); err != nil {
			a.logger.Warn("mongodb disconnect failed", "error", err)
		}
	}
	a.logger.Info("mongodb connected", "database", a.cfg.MongoDB.Database)

	executor := orders.Executor(orders.DryRunExecutor{DryRun: true})
	positionProvider := orders.PositionProvider(orders.EmptyPositionProvider{})
	if !a.cfg.App.DryRun {
		binanceExecutor := binanceexec.NewExecutor(binanceexec.ExecutorConfig{
			APIKey:               a.cfg.Binance.APIKey,
			APISecret:            a.cfg.Binance.APISecret,
			BaseURL:              a.cfg.Binance.FuturesBaseURL,
			Testnet:              a.cfg.Binance.Testnet,
			RealTradingEnabled:   a.cfg.App.RealTradingEnabled,
			RequestTimeout:       a.cfg.Binance.RequestTimeout,
			ExchangeInfoCacheTTL: a.cfg.Binance.ExchangeInfoCacheTTL,
		}, a.logger)
		executor = binanceExecutor
		positionProvider = binanceExecutor
	}

	orderService := orders.NewServiceWithRepositories(a.cfg.App.ConfirmationTTL, executor, orders.ServiceDependencies{
		ConfirmationStore: store,
		IntentStore:       store,
		AuditRecorder:     store,
	}, a.logger)
	statusService := orders.NewStatusService(positionProvider)
	planService := plans.NewService(store)
	return orderService, statusService, planService, store, cleanup, nil
}

func (a *App) newSignalProcessor(orderService *orders.Service, signalStore signals.SignalStore) *signals.Processor {
	var advisor signals.Advisor
	if a.cfg.AI.Enabled {
		switch a.cfg.AI.Provider {
		case "openai_compatible":
			advisor = ai.NewOpenAICompatibleAdvisor(ai.OpenAICompatibleConfig{
				APIKey:         a.cfg.AI.APIKey,
				BaseURL:        a.cfg.AI.BaseURL,
				Model:          a.cfg.AI.Model,
				SystemPrompt:   a.cfg.AI.SystemPrompt,
				RequestTimeout: a.cfg.AI.RequestTimeout,
			})
		default:
			a.logger.Warn("ai provider is not supported", "provider", a.cfg.AI.Provider)
		}
	}

	adminUserID := a.cfg.Telegram.AdminUserID
	if adminUserID == 0 && len(a.cfg.Telegram.AllowedUserIDs) > 0 {
		adminUserID = a.cfg.Telegram.AllowedUserIDs[0]
	}
	if adminUserID == 0 {
		a.logger.Warn("signal processor has no admin user id")
	}

	return signals.NewProcessor(signals.ProcessorConfig{
		Advisor:              advisor,
		OrderService:         orderService,
		SignalStore:          signalStore,
		AdminUserID:          adminUserID,
		MaxLeverage:          a.cfg.App.MaxLeverage,
		MinConfidencePercent: a.cfg.AI.MinConfidencePercent,
		AutoTradeEnabled:     a.cfg.AI.AutoTradeEnabled,
		Logger:               a.logger,
	})
}
