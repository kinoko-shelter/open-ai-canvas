package service

import (
	"testing"

	"infinite-canvas/backend/internal/model"
)

func TestRequireAigcProjectManagerRequiresActiveAdmin(t *testing.T) {
	svc := &Service{}
	tests := []struct {
		name    string
		user    *model.User
		wantErr bool
	}{
		{name: "active admin", user: &model.User{Role: model.UserRoleAdmin, Status: model.UserStatusActive}},
		{name: "team lead", user: &model.User{Role: model.UserRoleTeamLead, Status: model.UserStatusActive}, wantErr: true},
		{name: "disabled admin", user: &model.User{Role: model.UserRoleAdmin, Status: model.UserStatusDisabled}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := svc.RequireAigcProjectManager(test.user); (err != nil) != test.wantErr {
				t.Fatalf("RequireAigcProjectManager() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}
