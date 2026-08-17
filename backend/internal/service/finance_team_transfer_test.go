package service

import (
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTransferTeamCreditsMovesBalanceAndIsIdempotent(t *testing.T) {
	svc, db, lead, member, _ := teamCreditTransferFixture(t)
	amount := int64(2 * CreditScale)
	req := TeamCreditTransferRequest{UserID: member.ID, AmountMicrocredits: amount, Note: "本周项目额度", IdempotencyKey: "team-transfer-001"}

	result, err := svc.TransferTeamCredits(lead, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.SenderAccount.AvailableMicrocredits != 3*CreditScale || result.RecipientAccount.AvailableMicrocredits != amount {
		t.Fatalf("transfer result = %#v", result)
	}

	replayed, err := svc.TransferTeamCredits(lead, req)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.SenderAccount.AvailableMicrocredits != 3*CreditScale || replayed.RecipientAccount.AvailableMicrocredits != amount {
		t.Fatalf("replayed result = %#v", replayed)
	}

	var entries []model.CreditLedgerEntry
	if err := db.Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("ledger entries = %d, want 2", len(entries))
	}
	byType := make(map[model.CreditLedgerType]model.CreditLedgerEntry, len(entries))
	for _, entry := range entries {
		byType[entry.Type] = entry
	}
	if debit := byType[model.CreditLedgerTransferOut]; debit.UserID != lead.ID || debit.AmountMicrocredits != -amount || debit.AvailableAfterMicrocredits != 3*CreditScale {
		t.Fatalf("debit ledger = %#v", debit)
	}
	if credit := byType[model.CreditLedgerTransferIn]; credit.UserID != member.ID || credit.AmountMicrocredits != amount || credit.AvailableAfterMicrocredits != amount {
		t.Fatalf("credit ledger = %#v", credit)
	}

	filtered, total, err := svc.repo.CreditLedger(lead.ID, "transfer", 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(filtered) != 1 || filtered[0].Type != model.CreditLedgerTransferOut {
		t.Fatalf("transfer ledger = %#v, total=%d", filtered, total)
	}
}

func TestTransferTeamCreditsRestrictsRecipientsAndFunds(t *testing.T) {
	svc, db, lead, member, deptID := teamCreditTransferFixture(t)
	otherDeptID := deptID + 1
	if err := db.Create(&model.AigcDepartment{DeptID: otherDeptID, Name: "其他团队", Status: "启用"}).Error; err != nil {
		t.Fatal(err)
	}
	otherMember := &model.User{ID: "member-other", Username: "other-member", DisplayName: "Other Member", Role: model.UserRoleTeamMember, Status: model.UserStatusActive, DeptID: &otherDeptID}
	disabledMember := &model.User{ID: "member-disabled", Username: "disabled-member", DisplayName: "Disabled Member", Role: model.UserRoleTeamMember, Status: model.UserStatusDisabled, DeptID: &deptID}
	if err := db.Create(&[]model.User{*otherMember, *disabledMember}).Error; err != nil {
		t.Fatal(err)
	}

	recipients, err := svc.TeamCreditRecipients(lead)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 || recipients[0].ID != member.ID {
		t.Fatalf("team recipients = %#v", recipients)
	}
	if _, err := svc.TransferTeamCredits(member, TeamCreditTransferRequest{UserID: lead.ID, AmountMicrocredits: CreditScale, Note: "越权", IdempotencyKey: "member-transfer"}); err == nil {
		t.Fatal("team member unexpectedly transferred credits")
	}

	for _, req := range []TeamCreditTransferRequest{
		{UserID: otherMember.ID, AmountMicrocredits: CreditScale, Note: "跨团队", IdempotencyKey: "cross-team"},
		{UserID: disabledMember.ID, AmountMicrocredits: CreditScale, Note: "停用成员", IdempotencyKey: "disabled-member"},
		{UserID: member.ID, AmountMicrocredits: 6 * CreditScale, Note: "余额不足", IdempotencyKey: "insufficient-funds"},
	} {
		if _, err := svc.TransferTeamCredits(lead, req); err == nil {
			t.Fatalf("TransferTeamCredits(%+v) unexpectedly succeeded", req)
		}
	}

	account, err := svc.repo.CreditAccount(lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicrocredits != 5*CreditScale {
		t.Fatalf("lead balance after rejected transfers = %d", account.AvailableMicrocredits)
	}
	var entryCount int64
	if err := db.Model(&model.CreditLedgerEntry{}).Count(&entryCount).Error; err != nil {
		t.Fatal(err)
	}
	if entryCount != 0 {
		t.Fatalf("rejected transfers left %d ledger entries", entryCount)
	}
}

func teamCreditTransferFixture(t *testing.T) (*Service, *gorm.DB, *model.User, *model.User, int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.AigcDepartment{}, &model.User{}, &model.CreditAccount{}, &model.CreditLedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	deptID := int64(101)
	if err := db.Create(&model.AigcDepartment{DeptID: deptID, Name: "创作团队", Status: "启用"}).Error; err != nil {
		t.Fatal(err)
	}
	lead := &model.User{ID: "lead-1", Username: "lead", DisplayName: "Team Lead", Role: model.UserRoleTeamLead, Status: model.UserStatusActive, DeptID: &deptID}
	member := &model.User{ID: "member-1", Username: "member", DisplayName: "Team Member", Role: model.UserRoleTeamMember, Status: model.UserStatusActive, DeptID: &deptID}
	if err := db.Create(&[]model.User{*lead, *member}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CreditAccount{UserID: lead.ID, AvailableMicrocredits: 5 * CreditScale}).Error; err != nil {
		t.Fatal(err)
	}
	return &Service{repo: repository.New(db)}, db, lead, member, deptID
}
