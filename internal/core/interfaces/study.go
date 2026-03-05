package interfaces

import (
	"context"

	"github.com/shopspring/decimal"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// Study represents an analytics plugin that computes real-time metrics.
type Study interface {
	Plugin
	StartAsync(ctx context.Context) error
	StopAsync(ctx context.Context) error

	// TileTitle returns the display title for the study tile.
	TileTitle() string
	// TileToolTip returns the tooltip text for the study tile.
	TileToolTip() string

	// OnCalculated returns a channel that emits calculated study values.
	OnCalculated() <-chan models.BaseStudyModel
	// OnAlertTriggered returns a channel that emits alert values.
	OnAlertTriggered() <-chan decimal.Decimal
}
