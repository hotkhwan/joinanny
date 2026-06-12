package domain

import "bottrade/internal/decimal"

type PositionSide string

const (
	PositionSideLong  PositionSide = "long"
	PositionSideShort PositionSide = "short"
)

type Position struct {
	Symbol           string
	Side             PositionSide
	Amount           decimal.Decimal
	EntryPrice       decimal.Decimal
	MarkPrice        decimal.Decimal
	UnrealizedProfit decimal.Decimal
	Leverage         int
	MarginType       string
}
