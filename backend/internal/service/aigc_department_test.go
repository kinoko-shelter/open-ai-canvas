package service

import (
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAigcDepartmentsLimitsTeamLeadToOwnTeam(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AigcDepartment{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Create(&[]model.AigcDepartment{
		{DeptID: 107, Name: "运营团队", Status: "启用", CreatedAt: now, UpdatedAt: now},
		{DeptID: 108, Name: "研发团队", Status: "启用", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatal(err)
	}
	deptID := int64(107)
	lead := &model.User{ID: "team-lead", Username: "team-lead", Role: model.UserRoleTeamLead, Status: model.UserStatusActive, DeptID: &deptID, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(lead).Error; err != nil {
		t.Fatal(err)
	}

	departments, err := (&Service{repo: repository.New(db)}).AigcDepartments(lead, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(departments) != 1 || departments[0].DeptID != deptID || departments[0].Name != "运营团队" {
		t.Fatalf("team lead departments = %#v", departments)
	}
}
