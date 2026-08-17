package service

import (
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAdminTransferTeamCreditsUsesLeadBalanceAndWritesOneAudit(t *testing.T) {
	db, svc, primary, secondary, lead, member := adminTeamCreditTransferFixture(t)

	overview, err := svc.AdminTeamCreditOverview(primary)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Leads) != 1 || overview.Leads[0].ID != lead.ID || !overview.Leads[0].CanTransfer || len(overview.Leads[0].Members) != 1 || overview.Leads[0].Members[0].ID != member.ID {
		t.Fatalf("team credit overview = %#v", overview)
	}

	req := AdminTeamCreditTransferRequest{
		SenderUserID: lead.ID, RecipientUserID: member.ID, AmountMicrocredits: 2 * CreditScale,
		Note: "本周项目额度", IdempotencyKey: "admin-team-transfer-001",
	}
	result, err := svc.AdminTransferTeamCredits(primary, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.SenderAccount.AvailableMicrocredits != 3*CreditScale || result.RecipientAccount.AvailableMicrocredits != 2*CreditScale {
		t.Fatalf("admin transfer result = %#v", result)
	}

	replayed, err := svc.AdminTransferTeamCredits(primary, req)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.SenderAccount.AvailableMicrocredits != 3*CreditScale || replayed.RecipientAccount.AvailableMicrocredits != 2*CreditScale {
		t.Fatalf("replayed admin transfer result = %#v", replayed)
	}

	var auditEvents []model.AdminAuditEvent
	if err := db.Where("action = ?", "credits.team_transfer").Find(&auditEvents).Error; err != nil {
		t.Fatal(err)
	}
	if len(auditEvents) != 1 || auditEvents[0].ActorUserID != primary.ID || !strings.Contains(auditEvents[0].MetadataJSON, lead.ID) || !strings.Contains(auditEvents[0].MetadataJSON, member.ID) {
		t.Fatalf("admin transfer audits = %#v", auditEvents)
	}

	if _, err := svc.AdminTransferTeamCredits(secondary, AdminTeamCreditTransferRequest{
		SenderUserID: lead.ID, RecipientUserID: member.ID, AmountMicrocredits: CreditScale,
		Note: "secondary admin", IdempotencyKey: "secondary-admin-transfer",
	}); err == nil {
		t.Fatal("secondary admin unexpectedly transferred team credits")
	}
}

func TestAdminTeamCreditOverviewUsesEmptyMemberArray(t *testing.T) {
	db, svc, primary, _, _, _ := adminTeamCreditTransferFixture(t)
	deptID := int64(502)
	if err := db.Create(&model.AigcDepartment{DeptID: deptID, Name: "空团队", Status: "启用"}).Error; err != nil {
		t.Fatal(err)
	}
	lead := model.User{ID: "empty-team-lead", Username: "empty-team-lead", DisplayName: "Empty Team Lead", Role: model.UserRoleTeamLead, Status: model.UserStatusActive, DeptID: &deptID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&lead).Error; err != nil {
		t.Fatal(err)
	}

	overview, err := svc.AdminTeamCreditOverview(primary)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range overview.Leads {
		if item.ID == lead.ID {
			if item.Members == nil || len(item.Members) != 0 || item.CanTransfer {
				t.Fatalf("empty-team lead = %#v", item)
			}
			return
		}
	}
	t.Fatal("empty-team lead missing from overview")
}

func adminTeamCreditTransferFixture(t *testing.T) (*gorm.DB, *Service, *model.User, *model.User, *model.User, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.AigcDepartment{}, &model.User{}, &model.CreditAccount{}, &model.CreditLedgerEntry{}, &model.AdminAuditEvent{}); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	deptID := int64(501)
	if err := db.Create(&model.AigcDepartment{DeptID: deptID, Name: "创作团队", Status: "启用"}).Error; err != nil {
		t.Fatal(err)
	}
	primary := &model.User{ID: "admin-primary", Username: "primary-admin", DisplayName: "Primary Admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive, CreatedAt: createdAt, UpdatedAt: createdAt}
	secondary := &model.User{ID: "admin-secondary", Username: "secondary-admin", DisplayName: "Secondary Admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive, CreatedAt: createdAt.Add(time.Second), UpdatedAt: createdAt.Add(time.Second)}
	lead := &model.User{ID: "team-lead", Username: "team-lead", DisplayName: "Team Lead", Role: model.UserRoleTeamLead, Status: model.UserStatusActive, DeptID: &deptID, CreatedAt: createdAt.Add(2 * time.Second), UpdatedAt: createdAt.Add(2 * time.Second)}
	member := &model.User{ID: "team-member", Username: "team-member", DisplayName: "Team Member", Role: model.UserRoleTeamMember, Status: model.UserStatusActive, DeptID: &deptID, CreatedAt: createdAt.Add(3 * time.Second), UpdatedAt: createdAt.Add(3 * time.Second)}
	if err := db.Create(&[]model.User{*primary, *secondary, *lead, *member}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CreditAccount{UserID: lead.ID, AvailableMicrocredits: 5 * CreditScale}).Error; err != nil {
		t.Fatal(err)
	}
	return db, &Service{repo: repository.New(db)}, primary, secondary, lead, member
}
