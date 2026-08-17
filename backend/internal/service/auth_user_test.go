package service

import (
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPublicAuthUserIncludesLinuxDOIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserIdentity{}, &model.AigcDepartment{}); err != nil {
		t.Fatal(err)
	}
	deptID := int64(103)
	user := model.User{ID: "user-1", Username: "canvas-user", DisplayName: "Canvas User", DeptID: &deptID, Role: model.UserRoleUser, Status: model.UserStatusActive}
	department := model.AigcDepartment{DeptID: deptID, Name: "AIGC Team", Status: "启用"}
	identity := model.UserIdentity{ID: "identity-1", UserID: user.ID, Provider: "linuxdo", Subject: "123456", ProviderUsername: "linux-user", AvatarURL: "https://example.com/avatar.png"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatal(err)
	}

	result, err := (&Service{repo: repository.New(db)}).PublicAuthUser(&user)
	if err != nil {
		t.Fatal(err)
	}
	if result.AvatarURL != identity.AvatarURL || result.IdentityProvider != "linuxdo" || result.IdentityID != identity.Subject || result.IdentityUsername != identity.ProviderUsername {
		t.Fatalf("PublicAuthUser() = %#v", result)
	}
	if result.DeptName != department.Name {
		t.Fatalf("PublicAuthUser().DeptName = %q, want %q", result.DeptName, department.Name)
	}
}

func TestPublicAuthUserKeepsLocalUserWithoutIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserIdentity{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-1", Username: "local-user", DisplayName: "Local User"}

	result, err := (&Service{repo: repository.New(db)}).PublicAuthUser(&user)
	if err != nil {
		t.Fatal(err)
	}
	if result.Username != user.Username || result.AvatarURL != "" || result.IdentityProvider != "" || result.IdentityID != "" {
		t.Fatalf("PublicAuthUser() = %#v", result)
	}
	if result.DeptID != nil || result.DeptName != "" {
		t.Fatalf("PublicAuthUser() team fields = deptId:%v deptName:%q", result.DeptID, result.DeptName)
	}
}

func TestPublicAuthUserOmitsMissingDepartmentName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserIdentity{}, &model.AigcDepartment{}); err != nil {
		t.Fatal(err)
	}
	deptID := int64(404)
	user := model.User{ID: "user-1", Username: "local-user", DisplayName: "Local User", DeptID: &deptID}

	result, err := (&Service{repo: repository.New(db)}).PublicAuthUser(&user)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeptID == nil || *result.DeptID != deptID || result.DeptName != "" {
		t.Fatalf("PublicAuthUser() team fields = deptId:%v deptName:%q", result.DeptID, result.DeptName)
	}
}
