package repository

import (
	"errors"
	"testing"

	"infinite-canvas/backend/internal/model"
)

func TestTokenUsageAmountRequiresArkVideoOutputTokens(t *testing.T) {
	order := model.BillingOrder{
		Capability:                   "video",
		OutputTokenPriceMicrocredits: 1_000_000,
		MultiplierBasisPoints:        10_000,
	}
	_, err := tokenUsageAmount(order, &BillingUsage{})
	if !errors.Is(err, ErrBillingUsageUnavailable) {
		t.Fatalf("tokenUsageAmount() error = %v, want ErrBillingUsageUnavailable", err)
	}
}
