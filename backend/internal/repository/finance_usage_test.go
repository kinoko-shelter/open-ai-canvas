package repository

import (
	"errors"
	"testing"

	"infinite-canvas/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func TestBillingOrderAigcProjectIDPropagatesToLedgers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.CreditAccount{}, &model.BillingOrder{}, &model.CreditLedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	projectID := int64(42)

	settleOrder := billingOrderWithProject("order-settle", "idem-settle", &projectID)
	if err := db.Create(&model.CreditAccount{UserID: settleOrder.UserID, AvailableMicrocredits: 1_000}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.ReserveBillingOrder(settleOrder); err != nil {
		t.Fatal(err)
	}
	if err := repo.SettleBillingOrder(settleOrder.ID, ""); err != nil {
		t.Fatal(err)
	}

	refundOrder := billingOrderWithProject("order-refund", "idem-refund", &projectID)
	if err := repo.ReserveBillingOrder(refundOrder); err != nil {
		t.Fatal(err)
	}
	if err := repo.RefundBillingOrder(refundOrder.ID, "refund"); err != nil {
		t.Fatal(err)
	}

	var entries []model.CreditLedgerEntry
	if err := db.Where("billing_order_id IN ?", []string{settleOrder.ID, refundOrder.ID}).Order("created_at asc").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("ledger count = %d, want 4", len(entries))
	}
	for _, entry := range entries {
		if entry.AigcProjectID == nil || *entry.AigcProjectID != projectID {
			t.Fatalf("ledger %#v did not inherit project ID %d", entry, projectID)
		}
	}
}

func billingOrderWithProject(id string, idempotencyKey string, projectID *int64) *model.BillingOrder {
	return &model.BillingOrder{
		ID:                         id,
		UserID:                     "user-1",
		IdempotencyKey:             idempotencyKey,
		AigcProjectID:              projectID,
		ChannelID:                  "channel-1",
		ChannelModelID:             "channel-model-1",
		Model:                      "text-model",
		Capability:                 "text",
		Scene:                      "text",
		BillingMode:                "fixed_request",
		UnitPriceMicrocredits:      100,
		MultiplierBasisPoints:      10_000,
		Quantity:                   1,
		AmountMicrocredits:         100,
		ReservedAmountMicrocredits: 100,
		Status:                     model.BillingStatusReserved,
	}
}
