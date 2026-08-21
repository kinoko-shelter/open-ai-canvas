package service

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAssetFromJSONAcceptsDeterministicGenerationID(t *testing.T) {
	id := "generation_" + strings.Repeat("a", 64)
	raw, err := json.Marshal(map[string]any{"id": id, "kind": "image", "title": "生成图片"})
	if err != nil {
		t.Fatalf("marshal asset: %v", err)
	}

	asset, err := assetFromJSON("user-1", raw)
	if err != nil {
		t.Fatalf("assetFromJSON: %v", err)
	}
	if asset.ID != id {
		t.Fatalf("asset ID = %q, want %q", asset.ID, id)
	}
	if len([]rune(asset.ID)) > model.AssetIDMaxLength {
		t.Fatalf("generation asset ID length = %d, limit %d", len([]rune(asset.ID)), model.AssetIDMaxLength)
	}
}

func TestAssetFromJSONRejectsIDOverLimit(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"id": strings.Repeat("a", model.AssetIDMaxLength+1), "kind": "image"})
	if err != nil {
		t.Fatalf("marshal asset: %v", err)
	}

	_, err = assetFromJSON("user-1", raw)
	if err == nil || !strings.Contains(err.Error(), "素材 ID 不能超过 80 个字符") {
		t.Fatalf("assetFromJSON error = %v", err)
	}
}

func TestAssetFromJSONRejectsPrimaryVersionIDOverLimit(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"id": "asset-1", "primaryVersionId": strings.Repeat("v", 37)})
	if err != nil {
		t.Fatalf("marshal asset: %v", err)
	}

	_, err = assetFromJSON("user-1", raw)
	if err == nil || !strings.Contains(err.Error(), "素材主版本 ID 不能超过 36 个字符") {
		t.Fatalf("assetFromJSON error = %v", err)
	}
}

func TestValidateSyncedPayloadAllowsDataURLMentionInErrorMessage(t *testing.T) {
	raw, err := json.Marshal(map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{
				"metadata": map[string]interface{}{
					"errorDetails": "Expected a base64 image such as data:image/png;base64,aW1n, but received application/octet-stream",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSyncedPayload(raw, "画布"); err != nil {
		t.Fatalf("validateSyncedPayload() error = %v", err)
	}
}

func TestValidateSyncedPayloadRejectsNestedInlineMedia(t *testing.T) {
	for _, content := range []string{
		"data:image/png;base64,aW1n",
		"  DATA:VIDEO/mp4;base64,dmlkZW8=",
		"data:audio/mpeg;base64,YXVkaW8=",
	} {
		raw, err := json.Marshal(map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{"metadata": map[string]interface{}{"content": content}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := validateSyncedPayload(raw, "画布"); err == nil {
			t.Fatalf("validateSyncedPayload(%q) error = nil", content)
		}
	}
}

func TestTeamDataRoleCanReadDepartmentUserData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.AigcDepartment{}, &model.Asset{}, &model.CanvasProject{}, &model.Project{}); err != nil {
		t.Fatal(err)
	}
	deptID := int64(107)
	otherDeptID := int64(108)
	now := time.Now()
	actor := model.User{ID: "zyf", Username: "zyf", Role: model.UserRoleOperationsManager, Status: model.UserStatusActive, DeptID: &deptID, CreatedAt: now, UpdatedAt: now}
	member := model.User{ID: "jamie", Username: "jamie", Role: model.UserRoleTeamMember, Status: model.UserStatusActive, DeptID: &deptID, CreatedAt: now, UpdatedAt: now}
	other := model.User{ID: "other", Username: "other", Role: model.UserRoleTeamMember, Status: model.UserStatusActive, DeptID: &otherDeptID, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&[]model.User{actor, member, other}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.AigcDepartment{
		{DeptID: deptID, Name: "运营团队", Status: "启用", CreatedAt: now, UpdatedAt: now},
		{DeptID: otherDeptID, Name: "其他团队", Status: "启用", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Asset{
		{ID: "asset-own", UserID: actor.ID, Kind: "image", Title: "zyf 素材", PayloadJSON: `{"id":"asset-own","kind":"image","title":"zyf 素材"}`, CreatedAt: now, UpdatedAt: now},
		{ID: "asset-member", UserID: member.ID, Kind: "image", Title: "jamie 素材", PayloadJSON: `{"id":"asset-member","kind":"image","title":"jamie 素材"}`, CreatedAt: now, UpdatedAt: now},
		{ID: "asset-other", UserID: other.ID, Kind: "image", Title: "其他团队素材", PayloadJSON: `{"id":"asset-other","kind":"image","title":"其他团队素材"}`, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.CanvasProject{
		{ID: "canvas-own", UserID: actor.ID, Title: "zyf 画布", PayloadJSON: `{"id":"canvas-own","title":"zyf 画布"}`, CreatedAt: now, UpdatedAt: now},
		{ID: "canvas-member", UserID: member.ID, Title: "jamie 画布", PayloadJSON: `{"id":"canvas-member","title":"jamie 画布"}`, CreatedAt: now, UpdatedAt: now},
		{ID: "canvas-other", UserID: other.ID, Title: "其他团队画布", PayloadJSON: `{"id":"canvas-other","title":"其他团队画布"}`, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Project{
		{ID: "project-own", UserID: actor.ID, Name: "zyf 漫剧", Type: "short-drama", Status: model.ProjectStatusActive, CreatedAt: now, UpdatedAt: now},
		{ID: "project-member", UserID: member.ID, Name: "jamie 漫剧", Type: "short-drama", Status: model.ProjectStatusActive, CreatedAt: now, UpdatedAt: now},
		{ID: "project-other", UserID: other.ID, Name: "其他团队漫剧", Type: "short-drama", Status: model.ProjectStatusActive, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := &Service{repo: repository.New(db)}
	assets, err := svc.UserAssetSummariesForUser(&actor)
	if err != nil {
		t.Fatal(err)
	}
	if gotIDs(assets) != "asset-member,asset-own" {
		t.Fatalf("asset IDs = %s", gotIDs(assets))
	}
	assertSummaryOwner(t, assets, "asset-member", "jamie", "运营团队")
	canvases, err := svc.UserCanvasProjectSummariesForUser(&actor)
	if err != nil {
		t.Fatal(err)
	}
	if gotIDs(canvases) != "canvas-member,canvas-own" {
		t.Fatalf("canvas IDs = %s", gotIDs(canvases))
	}
	assertSummaryOwner(t, canvases, "canvas-member", "jamie", "运营团队")
	projects, err := svc.ListProjects(actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	projectIDs := make([]UserDataSummary, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, UserDataSummary{ID: project.Project.ID})
	}
	if gotIDs(projectIDs) != "project-member,project-own" {
		t.Fatalf("project IDs = %s", gotIDs(projectIDs))
	}
	assertProjectOwner(t, projects, "project-member", "jamie", "运营团队")
}

func gotIDs(items []UserDataSummary) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func assertSummaryOwner(t *testing.T, items []UserDataSummary, id string, username string, deptName string) {
	t.Helper()
	for _, item := range items {
		if item.ID != id {
			continue
		}
		if item.CreatorUsername != username || item.DeptName != deptName {
			t.Fatalf("%s owner = %s/%s", id, item.CreatorUsername, item.DeptName)
		}
		return
	}
	t.Fatalf("summary %s not found", id)
}

func assertProjectOwner(t *testing.T, items []ProjectSummary, id string, username string, deptName string) {
	t.Helper()
	for _, item := range items {
		if item.Project.ID != id {
			continue
		}
		if item.CreatorUsername != username || item.DeptName != deptName {
			t.Fatalf("%s owner = %s/%s", id, item.CreatorUsername, item.DeptName)
		}
		return
	}
	t.Fatalf("project %s not found", id)
}
