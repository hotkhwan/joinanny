package app

import (
	"context"
	"errors"
	"testing"

	"bottrade/internal/config"
)

// Each role entrypoint must check the context before opening any external
// connection (MongoDB, Telegram, HTTP). A pre-cancelled context therefore
// returns immediately with the context error and never touches the network,
// which is what lets these run without a database in tests.
func TestRunRoles_ReturnOnCancelledContext(t *testing.T) {
	roles := map[string]func(*App, context.Context) error{
		"worker": (*App).RunWorker,
		"api":    (*App).RunAPI,
		"all":    (*App).Run,
	}

	for name, run := range roles {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			a := New(config.Config{}, nil)
			err := run(a, ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("role %s: expected context.Canceled, got %v", name, err)
			}
		})
	}
}
