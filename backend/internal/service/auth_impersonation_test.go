package service

import (
	"errors"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/gorm"
)

func TestUserImpersonationOnlyAllowsActiveNormalUsers(t *testing.T) {
	db := newBulkUserTestDB(t)
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	actor := model.User{ID: "admin-1", Username: "admin-one", DisplayName: "Admin One", Role: model.UserRoleAdmin, Status: model.UserStatusActive, CreatedAt: createdAt, UpdatedAt: createdAt}
	target := model.User{ID: "user-1", Username: "user-one", DisplayName: "User One", Role: model.UserRoleUser, Status: model.UserStatusActive, CreatedAt: createdAt.Add(time.Second), UpdatedAt: createdAt.Add(time.Second)}
	otherAdmin := model.User{ID: "admin-2", Username: "admin-two", Role: model.UserRoleAdmin, Status: model.UserStatusActive, CreatedAt: createdAt.Add(2 * time.Second), UpdatedAt: createdAt.Add(2 * time.Second)}
	disabledUser := model.User{ID: "user-2", Username: "user-two", Role: model.UserRoleUser, Status: model.UserStatusDisabled, CreatedAt: createdAt.Add(3 * time.Second), UpdatedAt: createdAt.Add(3 * time.Second)}
	if err := db.Create(&[]model.User{actor, target, otherAdmin, disabledUser}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repository.New(db)}
	if allowed, err := svc.CanImpersonateUsers(&actor); err != nil || !allowed {
		t.Fatalf("primary admin capability = %t, %v", allowed, err)
	}
	if allowed, err := svc.CanImpersonateUsers(&otherAdmin); err != nil || allowed {
		t.Fatalf("secondary admin capability = %t, %v", allowed, err)
	}

	adminSession, err := svc.createAuthSession(&actor)
	if err != nil {
		t.Fatal(err)
	}
	impersonation, err := svc.StartUserImpersonation(adminSession.Session, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	context, err := svc.CurrentAuthSession(impersonation.Session)
	if err != nil {
		t.Fatal(err)
	}
	if context.User.ID != target.ID || context.Impersonator == nil || context.Impersonator.ID != actor.ID {
		t.Fatalf("impersonation context = %#v", context)
	}
	originalSessionID, _ := parseSessionCookie(adminSession.Session)
	if _, err := svc.repo.AuthSession(originalSessionID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("original admin session error = %v, want record not found", err)
	}

	returned, err := svc.ExitUserImpersonation(impersonation.Session)
	if err != nil {
		t.Fatal(err)
	}
	returnedContext, err := svc.CurrentAuthSession(returned.Session)
	if err != nil {
		t.Fatal(err)
	}
	if returnedContext.User.ID != actor.ID || returnedContext.Impersonator != nil {
		t.Fatalf("returned context = %#v", returnedContext)
	}

	for _, targetID := range []string{actor.ID, otherAdmin.ID, disabledUser.ID} {
		freshAdminSession, err := svc.createAuthSession(&actor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.StartUserImpersonation(freshAdminSession.Session, targetID); err == nil {
			t.Fatalf("StartUserImpersonation(%q) error = nil", targetID)
		}
	}
	secondaryAdminSession, err := svc.createAuthSession(&otherAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartUserImpersonation(secondaryAdminSession.Session, target.ID); err == nil {
		t.Fatal("secondary admin was allowed to impersonate a user")
	}

	var events []model.AdminAuditEvent
	if err := db.Where("target_type = ? AND target_id = ?", "user", target.ID).Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].ActorUserID != actor.ID || events[1].ActorUserID != actor.ID {
		t.Fatalf("audit events = %#v", events)
	}
	actions := map[string]bool{}
	for _, event := range events {
		actions[event.Action] = true
	}
	if !actions["user.impersonation.start"] || !actions["user.impersonation.exit"] {
		t.Fatalf("audit actions = %#v", actions)
	}
}
