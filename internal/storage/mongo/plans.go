package mongo

import (
	"context"
	"fmt"

	"bottrade/internal/plans"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (s *Store) PlanStatus(ctx context.Context, userID int64, planID string) (plans.Status, error) {
	filter := bson.D{
		{Key: "user_id", Value: userID},
		{Key: "plan_id", Value: planID},
	}
	cursor, err := s.orderIntents.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(50))
	if err != nil {
		return plans.Status{}, fmt.Errorf("find plan intents: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []orderIntentDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return plans.Status{}, fmt.Errorf("decode plan intents: %w", err)
	}

	status := plans.Status{
		PlanID:      planID,
		IntentCount: len(docs),
	}
	seen := map[string]struct{}{}
	for i, doc := range docs {
		if i == 0 {
			status.LastUpdated = doc.UpdatedAt
		}
		if doc.Symbol == "" {
			continue
		}
		if _, ok := seen[doc.Symbol]; ok {
			continue
		}
		seen[doc.Symbol] = struct{}{}
		status.Symbols = append(status.Symbols, doc.Symbol)
	}

	return status, nil
}
