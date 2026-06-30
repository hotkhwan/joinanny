package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
	"bottrade/internal/orders"
)

type ScheduledCloseStatus string

const (
	ScheduledCloseStatusPending   ScheduledCloseStatus = "pending"
	ScheduledCloseStatusExecuting ScheduledCloseStatus = "executing"
	ScheduledCloseStatusDone      ScheduledCloseStatus = "done"
	ScheduledCloseStatusCancelled ScheduledCloseStatus = "cancelled"
	ScheduledCloseStatusSkipped   ScheduledCloseStatus = "skipped"
)

const scheduledClosePollInterval = 30 * time.Second

// ScheduledCloseClaimTimeout lets another API instance recover a close job if
// the process dies after claiming it but before writing a terminal status.
const ScheduledCloseClaimTimeout = 5 * time.Minute

// ScheduledClose is the durable plan-end close job for a testnet Mission. It is
// protective plumbing: the poller closes an open position after the plan window
// survives API restarts.
type ScheduledClose struct {
	ID             string               `json:"id" bson:"_id"`
	UserKey        string               `json:"-" bson:"user_key"`
	UserID         int64                `json:"-" bson:"user_id"`
	Symbol         string               `json:"symbol" bson:"symbol"`
	DueAt          time.Time            `json:"due_at" bson:"due_at"`
	Status         ScheduledCloseStatus `json:"status" bson:"status"`
	ConfirmationID string               `json:"confirmation_id,omitempty" bson:"confirmation_id,omitempty"`
	Reason         string               `json:"reason,omitempty" bson:"reason,omitempty"`
	CreatedAt      time.Time            `json:"created_at" bson:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at" bson:"updated_at"`
	PurgeAt        *time.Time           `json:"purge_at,omitempty" bson:"purge_at,omitempty"`
}

type ScheduledCloseStore interface {
	Save(ctx context.Context, close ScheduledClose) error
	ListDue(ctx context.Context, now time.Time, limit int) ([]ScheduledClose, error)
	ClaimDue(ctx context.Context, id string, now time.Time) (ScheduledClose, bool, error)
	MarkDone(ctx context.Context, id, confirmationID, reason string, now time.Time) (ScheduledClose, bool, error)
	MarkSkipped(ctx context.Context, id, confirmationID, reason string, now time.Time) (ScheduledClose, bool, error)
	MarkCancelled(ctx context.Context, id, reason string, now time.Time) (ScheduledClose, bool, error)
}

type memScheduledCloses struct {
	mu   sync.Mutex
	rows map[string]ScheduledClose
}

func newMemScheduledCloses() *memScheduledCloses {
	return &memScheduledCloses{rows: make(map[string]ScheduledClose)}
}

func (m *memScheduledCloses) Save(_ context.Context, close ScheduledClose) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[close.ID] = close
	return nil
}

func (m *memScheduledCloses) ListDue(_ context.Context, now time.Time, limit int) ([]ScheduledClose, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	out := make([]ScheduledClose, 0, len(m.rows))
	staleBefore := now.Add(-ScheduledCloseClaimTimeout)
	for _, row := range m.rows {
		if row.DueAt.After(now) {
			continue
		}
		if row.Status == ScheduledCloseStatusPending ||
			(row.Status == ScheduledCloseStatusExecuting && !row.UpdatedAt.After(staleBefore)) {
			out = append(out, row)
		}
	}
	sortScheduledCloses(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memScheduledCloses) ClaimDue(_ context.Context, id string, now time.Time) (ScheduledClose, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	stale := row.Status == ScheduledCloseStatusExecuting && !row.UpdatedAt.After(now.Add(-ScheduledCloseClaimTimeout))
	if !ok || row.DueAt.After(now) || (row.Status != ScheduledCloseStatusPending && !stale) {
		return row, false, nil
	}
	row.Status = ScheduledCloseStatusExecuting
	row.UpdatedAt = now
	m.rows[id] = row
	return row, true, nil
}

func (m *memScheduledCloses) MarkDone(_ context.Context, id, confirmationID, reason string, now time.Time) (ScheduledClose, bool, error) {
	return m.markTerminal(id, ScheduledCloseStatusDone, confirmationID, reason, now)
}

func (m *memScheduledCloses) MarkSkipped(_ context.Context, id, confirmationID, reason string, now time.Time) (ScheduledClose, bool, error) {
	return m.markTerminal(id, ScheduledCloseStatusSkipped, confirmationID, reason, now)
}

func (m *memScheduledCloses) MarkCancelled(_ context.Context, id, reason string, now time.Time) (ScheduledClose, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	if !ok || (row.Status != ScheduledCloseStatusPending && row.Status != ScheduledCloseStatusExecuting) {
		return row, false, nil
	}
	row.Status = ScheduledCloseStatusCancelled
	row.Reason = strings.TrimSpace(reason)
	row.UpdatedAt = now
	row.PurgeAt = scheduledClosePurgeAt(now)
	m.rows[id] = row
	return row, true, nil
}

func (m *memScheduledCloses) markTerminal(id string, status ScheduledCloseStatus, confirmationID, reason string, now time.Time) (ScheduledClose, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	if !ok || row.Status != ScheduledCloseStatusExecuting {
		return row, false, nil
	}
	row.Status = status
	row.ConfirmationID = confirmationID
	row.Reason = strings.TrimSpace(reason)
	row.UpdatedAt = now
	row.PurgeAt = scheduledClosePurgeAt(now)
	m.rows[id] = row
	return row, true, nil
}

func sortScheduledCloses(rows []ScheduledClose) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].DueAt.Before(rows[j-1].DueAt); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func newScheduledCloseID() (string, error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("create scheduled close id: %w", err)
	}
	return "close_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func scheduledClosePurgeAt(now time.Time) *time.Time {
	if now.IsZero() {
		return nil
	}
	purgeAt := now.Add(armedMissionRetention)
	return &purgeAt
}

func (s *Server) scheduleTimedMissionClose(m timedMission) (ScheduledClose, error) {
	if !s.armedMissionRuntimeAllowed() || m.Duration <= 0 {
		return ScheduledClose{}, nil
	}
	if s.scheduledCloses == nil {
		return ScheduledClose{}, fmt.Errorf("scheduled close store is not configured")
	}
	now := time.Now().UTC()
	id, err := newScheduledCloseID()
	if err != nil {
		return ScheduledClose{}, err
	}
	close := ScheduledClose{
		ID:        id,
		UserKey:   orders.TraderKey(m.UserID),
		UserID:    m.UserID,
		Symbol:    normalizeSymbol(m.Symbol),
		DueAt:     now.Add(m.Duration),
		Status:    ScheduledCloseStatusPending,
		Reason:    "mission timed close",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.scheduledCloses.Save(context.Background(), close); err != nil {
		return ScheduledClose{}, err
	}
	s.logger.Info("scheduled durable mission close", "id", close.ID, "user_id", close.UserID, "symbol", close.Symbol, "due_at", close.DueAt)
	return close, nil
}

func (s *Server) cancelScheduledClose(ctx context.Context, close ScheduledClose, reason string) {
	if close.ID == "" || s.scheduledCloses == nil {
		return
	}
	if _, _, err := s.scheduledCloses.MarkCancelled(ctx, close.ID, reason, time.Now().UTC()); err != nil {
		s.logger.Warn("scheduled close cancel failed", "id", close.ID, "error", err)
	}
}

func (s *Server) startScheduledClosePoller(ctx context.Context) {
	if s.scheduledCloses == nil {
		return
	}
	go func() {
		run := func() {
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if _, err := s.runDueScheduledCloses(checkCtx, time.Now().UTC()); err != nil {
				s.logger.Warn("scheduled close poll failed", "error", err)
			}
			cancel()
		}
		run()
		ticker := time.NewTicker(scheduledClosePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func (s *Server) runDueScheduledCloses(ctx context.Context, now time.Time) (int, error) {
	if s.scheduledCloses == nil {
		return 0, nil
	}
	rows, err := s.scheduledCloses.ListDue(ctx, now, 100)
	if err != nil {
		return 0, err
	}
	handled := 0
	for _, row := range rows {
		if _, ok, err := s.handleScheduledClose(ctx, row.ID, now); err != nil {
			s.logger.Warn("scheduled close failed", "id", row.ID, "symbol", row.Symbol, "error", err)
		} else if ok {
			handled++
		}
	}
	return handled, nil
}

func (s *Server) handleScheduledClose(ctx context.Context, id string, now time.Time) (ScheduledClose, bool, error) {
	close, claimed, err := s.scheduledCloses.ClaimDue(ctx, id, now)
	if err != nil || !claimed {
		return close, false, err
	}
	if !s.armedMissionRuntimeAllowed() {
		updated, _, err := s.scheduledCloses.MarkSkipped(ctx, close.ID, "", "gate closed before timed close", now)
		return updated, true, err
	}
	if !s.hasActiveKeyForSubject(ctx, close.UserKey) {
		updated, _, err := s.scheduledCloses.MarkSkipped(ctx, close.ID, "", "active testnet key missing", now)
		return updated, true, err
	}
	if s.orders == nil {
		updated, _, err := s.scheduledCloses.MarkSkipped(ctx, close.ID, "", "orders service unavailable", now)
		return updated, true, err
	}
	positions, err := s.orders.PositionsWithRequiredUserExecutor(ctx, close.UserID)
	if err != nil {
		reason := "positions failed: " + err.Error()
		updated, _, markErr := s.scheduledCloses.MarkSkipped(ctx, close.ID, "", reason, now)
		if markErr != nil {
			return updated, true, markErr
		}
		return updated, true, err
	}
	if !scheduledCloseHasOpenPosition(positions, close.Symbol) {
		updated, _, err := s.scheduledCloses.MarkDone(ctx, close.ID, "", "no open position", now)
		return updated, true, err
	}
	intent := domain.Intent{Type: domain.IntentClose, Close: &domain.CloseIntent{
		Symbol: close.Symbol, All: true, ResolvedPercent: decimal.NewFromInt(100),
	}}
	confirmation, err := s.orders.Prepare(ctx, close.UserID, intent)
	if err != nil {
		reason := "prepare close failed: " + err.Error()
		updated, _, markErr := s.scheduledCloses.MarkSkipped(ctx, close.ID, "", reason, now)
		if markErr != nil {
			return updated, true, markErr
		}
		return updated, true, err
	}
	if !s.armedMissionRuntimeAllowed() || !s.hasActiveKeyForSubject(ctx, close.UserKey) {
		_ = s.orders.Cancel(ctx, close.UserID, confirmation.ID)
		updated, _, err := s.scheduledCloses.MarkSkipped(ctx, close.ID, confirmation.ID, "gate closed before timed close confirm", time.Now().UTC())
		return updated, true, err
	}
	if _, err := s.orders.ConfirmWithRequiredUserExecutor(ctx, close.UserID, confirmation.ID); err != nil {
		reason := "confirm close failed: " + err.Error()
		updated, _, markErr := s.scheduledCloses.MarkSkipped(ctx, close.ID, confirmation.ID, reason, time.Now().UTC())
		if markErr != nil {
			return updated, true, markErr
		}
		return updated, true, err
	}
	updated, _, err := s.scheduledCloses.MarkDone(ctx, close.ID, confirmation.ID, "closed at plan deadline", time.Now().UTC())
	if err == nil {
		s.logger.Info("durable timed mission close executed", "id", close.ID, "user_id", close.UserID, "symbol", close.Symbol, "confirmation_id", confirmation.ID)
	}
	return updated, true, err
}

func scheduledCloseHasOpenPosition(positions []domain.Position, symbol string) bool {
	for _, p := range positions {
		if strings.EqualFold(p.Symbol, symbol) && !p.Amount.IsZero() {
			return true
		}
	}
	return false
}
