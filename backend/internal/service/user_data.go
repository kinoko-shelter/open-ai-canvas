package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

type AssetsSyncRequest struct {
	Assets []json.RawMessage `json:"assets"`
}

type CanvasProjectsSyncRequest struct {
	Projects []json.RawMessage `json:"projects"`
}

type UserDataSummary struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind,omitempty"`
	Category  string    `json:"category,omitempty"`
	Status    string    `json:"status,omitempty"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UserDataSnapshot struct {
	Assets   []json.RawMessage `json:"assets"`
	Projects []json.RawMessage `json:"projects"`
}

func (s *Service) UserDataSnapshot(userID string) (UserDataSnapshot, error) {
	return s.UserDataSnapshotForUser(&model.User{ID: userID})
}

func (s *Service) UserDataSnapshotForUser(user *model.User) (UserDataSnapshot, error) {
	assets, err := s.UserAssetsForUser(user)
	if err != nil {
		return UserDataSnapshot{}, err
	}
	projects, err := s.UserCanvasProjectsForUser(user)
	if err != nil {
		return UserDataSnapshot{}, err
	}
	return UserDataSnapshot{Assets: assets, Projects: projects}, nil
}

func (s *Service) UserAssetSummaries(userID string) ([]UserDataSummary, error) {
	return s.UserAssetSummariesForUser(&model.User{ID: userID})
}

func (s *Service) UserAssetSummariesForUser(user *model.User) ([]UserDataSummary, error) {
	scope, err := s.dataScope(user)
	if err != nil {
		return nil, err
	}
	assets, err := s.repo.AssetSummariesForScope(scope)
	if err != nil {
		return nil, err
	}
	result := make([]UserDataSummary, 0, len(assets))
	for _, asset := range assets {
		result = append(result, UserDataSummary{ID: asset.ID, Kind: asset.Kind, Category: string(asset.Category), Status: string(asset.Status), Title: asset.Title, CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt})
	}
	return result, nil
}

func (s *Service) UserAsset(userID string, id string) (json.RawMessage, error) {
	return s.UserAssetForUser(&model.User{ID: userID}, id)
}

func (s *Service) UserAssetForUser(user *model.User, id string) (json.RawMessage, error) {
	asset, err := s.scopedAsset(user, id)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(asset.PayloadJSON), nil
}

func (s *Service) UpsertUserAsset(userID string, raw json.RawMessage) (UserDataSummary, error) {
	return s.upsertUserAssetForOwner(&model.User{ID: userID}, userID, raw)
}

func (s *Service) UpsertUserAssetForUser(user *model.User, raw json.RawMessage) (UserDataSummary, error) {
	ownerID := user.ID
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &identity) == nil && strings.TrimSpace(identity.ID) != "" {
		if existing, err := s.scopedAsset(user, identity.ID); err == nil {
			ownerID = existing.UserID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return UserDataSummary{}, err
		}
	}
	return s.upsertUserAssetForOwner(user, ownerID, raw)
}

func (s *Service) upsertUserAssetForOwner(actor *model.User, ownerID string, raw json.RawMessage) (UserDataSummary, error) {
	if err := s.canAccessOwnedUser(actor, ownerID); err != nil {
		return UserDataSummary{}, err
	}
	asset, err := assetFromJSON(ownerID, raw)
	if err != nil {
		return UserDataSummary{}, err
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return UserDataSummary{}, err
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	existing, existingErr := s.repo.AssetForUser(ownerID, asset.ID)
	if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
		return UserDataSummary{}, existingErr
	}
	existingBytes := int64(0)
	if existing != nil {
		existingBytes = int64(len([]byte(existing.PayloadJSON)))
	}
	usage, err := s.repo.UserStorageUsage(ownerID)
	if err != nil {
		return UserDataSummary{}, err
	}
	if err := validateStructuredStorageQuotaWithPolicy(usage, "asset", errors.Is(existingErr, gorm.ErrRecordNotFound), int64(len(raw))-existingBytes, policy.Resource); err != nil {
		return UserDataSummary{}, err
	}
	if err := s.repo.UpsertAsset(&asset); err != nil {
		return UserDataSummary{}, err
	}
	if existingErr != nil {
		s.recordActivity(ownerID, "asset", 1)
	}
	return UserDataSummary{ID: asset.ID, Kind: asset.Kind, Category: string(asset.Category), Status: string(asset.Status), Title: asset.Title, CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt}, nil
}

func (s *Service) DeleteUserAsset(userID string, id string) error {
	return s.DeleteUserAssetForUser(&model.User{ID: userID}, id)
}

func (s *Service) DeleteUserAssetForUser(user *model.User, id string) error {
	asset, err := s.scopedAsset(user, id)
	if err != nil {
		return err
	}
	references, err := s.repo.AssetReferenceCount(id)
	if err != nil {
		return err
	}
	if references > 0 {
		return BadAuthRequest("素材仍被项目或镜头引用，请先解除引用")
	}
	return s.repo.DeleteAsset(asset.UserID, id)
}

func (s *Service) UserAssets(userID string) ([]json.RawMessage, error) {
	return s.UserAssetsForUser(&model.User{ID: userID})
}

func (s *Service) UserAssetsForUser(user *model.User) ([]json.RawMessage, error) {
	scope, err := s.dataScope(user)
	if err != nil {
		return nil, err
	}
	assets, err := s.repo.AssetsForScope(scope)
	if err != nil {
		return nil, err
	}
	result := make([]json.RawMessage, 0, len(assets))
	for _, asset := range assets {
		if strings.TrimSpace(asset.PayloadJSON) != "" {
			result = append(result, json.RawMessage(asset.PayloadJSON))
		}
	}
	return result, nil
}

func (s *Service) ReplaceUserAssets(userID string, req AssetsSyncRequest) ([]json.RawMessage, error) {
	return s.ReplaceUserAssetsForUser(&model.User{ID: userID}, req)
}

func (s *Service) ReplaceUserAssetsForUser(user *model.User, req AssetsSyncRequest) ([]json.RawMessage, error) {
	scope, err := s.dataScope(user)
	if err != nil {
		return nil, err
	}
	ownerID := user.ID
	assets := make([]model.Asset, 0, len(req.Assets))
	var totalBytes int64
	for _, raw := range req.Assets {
		item, err := assetFromJSON(ownerID, raw)
		if err != nil {
			return nil, err
		}
		assets = append(assets, item)
		totalBytes += int64(len(raw))
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, err
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	usage, err := s.repo.UserStorageUsage(ownerID)
	if err != nil {
		return nil, err
	}
	if err := validateStructuredReplacementQuotaWithPolicy(usage, "asset", len(assets), totalBytes, policy.Resource); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceAssets(ownerID, assets); err != nil {
		return nil, err
	}
	if len(assets) > 0 {
		s.recordActivity(ownerID, "asset", len(assets))
	}
	_ = scope
	return s.UserAssetsForUser(user)
}

func (s *Service) UserCanvasProjects(userID string) ([]json.RawMessage, error) {
	return s.UserCanvasProjectsForUser(&model.User{ID: userID})
}

func (s *Service) UserCanvasProjectsForUser(user *model.User) ([]json.RawMessage, error) {
	scope, err := s.dataScope(user)
	if err != nil {
		return nil, err
	}
	projects, err := s.repo.CanvasProjectsForScope(scope)
	if err != nil {
		return nil, err
	}
	result := make([]json.RawMessage, 0, len(projects))
	for _, project := range projects {
		if strings.TrimSpace(project.PayloadJSON) != "" {
			result = append(result, json.RawMessage(project.PayloadJSON))
		}
	}
	return result, nil
}

func (s *Service) UserCanvasProjectSummaries(userID string) ([]UserDataSummary, error) {
	return s.UserCanvasProjectSummariesForUser(&model.User{ID: userID})
}

func (s *Service) UserCanvasProjectSummariesForUser(user *model.User) ([]UserDataSummary, error) {
	scope, err := s.dataScope(user)
	if err != nil {
		return nil, err
	}
	projects, err := s.repo.CanvasProjectSummariesForScope(scope)
	if err != nil {
		return nil, err
	}
	result := make([]UserDataSummary, 0, len(projects))
	for _, project := range projects {
		result = append(result, UserDataSummary{ID: project.ID, Title: project.Title, CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt})
	}
	return result, nil
}

func (s *Service) UserCanvasProject(userID string, id string) (json.RawMessage, error) {
	return s.UserCanvasProjectForUser(&model.User{ID: userID}, id)
}

func (s *Service) UserCanvasProjectForUser(user *model.User, id string) (json.RawMessage, error) {
	project, err := s.scopedCanvasProject(user, id)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(project.PayloadJSON), nil
}

func (s *Service) UpsertUserCanvasProject(userID string, raw json.RawMessage) (UserDataSummary, error) {
	return s.upsertUserCanvasProjectForOwner(&model.User{ID: userID}, userID, raw)
}

func (s *Service) UpsertUserCanvasProjectForUser(user *model.User, raw json.RawMessage) (UserDataSummary, error) {
	ownerID := user.ID
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &identity) == nil && strings.TrimSpace(identity.ID) != "" {
		if existing, err := s.scopedCanvasProject(user, identity.ID); err == nil {
			ownerID = existing.UserID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return UserDataSummary{}, err
		}
	}
	return s.upsertUserCanvasProjectForOwner(user, ownerID, raw)
}

func (s *Service) upsertUserCanvasProjectForOwner(actor *model.User, ownerID string, raw json.RawMessage) (UserDataSummary, error) {
	if err := s.canAccessOwnedUser(actor, ownerID); err != nil {
		return UserDataSummary{}, err
	}
	project, err := canvasProjectFromJSON(ownerID, raw)
	if err != nil {
		return UserDataSummary{}, err
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return UserDataSummary{}, err
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	existing, existingErr := s.repo.CanvasProjectForUser(ownerID, project.ID)
	if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
		return UserDataSummary{}, existingErr
	}
	if err := s.inheritCanvasAigcProject(ownerID, &project); err != nil {
		return UserDataSummary{}, err
	}
	existingBytes := int64(0)
	if existing != nil {
		existingBytes = int64(len([]byte(existing.PayloadJSON)))
	}
	usage, err := s.repo.UserStorageUsage(ownerID)
	if err != nil {
		return UserDataSummary{}, err
	}
	if err := validateStructuredStorageQuotaWithPolicy(usage, "canvas", errors.Is(existingErr, gorm.ErrRecordNotFound), int64(len(raw))-existingBytes, policy.Resource); err != nil {
		return UserDataSummary{}, err
	}
	if err := s.repo.UpsertCanvasProject(&project); err != nil {
		return UserDataSummary{}, err
	}
	if existingErr != nil || existing.PayloadJSON != project.PayloadJSON || existing.Title != project.Title {
		s.recordActivity(ownerID, "canvas", 1)
	}
	return UserDataSummary{ID: project.ID, Title: project.Title, CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt}, nil
}

func (s *Service) DeleteUserCanvasProject(userID string, id string) error {
	return s.DeleteUserCanvasProjectForUser(&model.User{ID: userID}, id)
}

func (s *Service) DeleteUserCanvasProjectForUser(user *model.User, id string) error {
	project, err := s.scopedCanvasProject(user, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteCanvasShare(project.UserID, id); err != nil {
		return err
	}
	return s.repo.DeleteCanvasProject(project.UserID, id)
}

func (s *Service) ReplaceUserCanvasProjects(userID string, req CanvasProjectsSyncRequest) ([]json.RawMessage, error) {
	return s.ReplaceUserCanvasProjectsForUser(&model.User{ID: userID}, req)
}

func (s *Service) ReplaceUserCanvasProjectsForUser(user *model.User, req CanvasProjectsSyncRequest) ([]json.RawMessage, error) {
	ownerID := user.ID
	projects := make([]model.CanvasProject, 0, len(req.Projects))
	var totalBytes int64
	for _, raw := range req.Projects {
		item, err := canvasProjectFromJSON(ownerID, raw)
		if err != nil {
			return nil, err
		}
		if err := s.inheritCanvasAigcProject(ownerID, &item); err != nil {
			return nil, err
		}
		projects = append(projects, item)
		totalBytes += int64(len(raw))
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, err
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	usage, err := s.repo.UserStorageUsage(ownerID)
	if err != nil {
		return nil, err
	}
	if err := validateStructuredReplacementQuotaWithPolicy(usage, "canvas", len(projects), totalBytes, policy.Resource); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceCanvasProjects(ownerID, projects); err != nil {
		return nil, err
	}
	if len(projects) > 0 {
		s.recordActivity(ownerID, "canvas", 1)
	}
	return s.UserCanvasProjectsForUser(user)
}

func assetFromJSON(userID string, raw json.RawMessage) (model.Asset, error) {
	if err := validateSyncedPayload(raw, "素材"); err != nil {
		return model.Asset{}, err
	}
	var payload struct {
		ID               string `json:"id"`
		Kind             string `json:"kind"`
		Category         string `json:"category"`
		Status           string `json:"status"`
		PrimaryVersionID string `json:"primaryVersionId"`
		Title            string `json:"title"`
		CreatedAt        string `json:"createdAt"`
		UpdatedAt        string `json:"updatedAt"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return model.Asset{}, BadAuthRequest("素材数据格式错误")
	}
	now := time.Now()
	createdAt := parseClientTime(payload.CreatedAt, now)
	updatedAt := parseClientTime(payload.UpdatedAt, createdAt)
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		id = newID()
	}
	if utf8.RuneCountInString(id) > model.AssetIDMaxLength {
		return model.Asset{}, BadAuthRequest("素材 ID 不能超过 80 个字符")
	}
	primaryVersionID := strings.TrimSpace(payload.PrimaryVersionID)
	if utf8.RuneCountInString(primaryVersionID) > 36 {
		return model.Asset{}, BadAuthRequest("素材主版本 ID 不能超过 36 个字符")
	}
	category := model.AssetCategory(strings.TrimSpace(payload.Category))
	if category == "" {
		category = model.AssetCategoryOther
	}
	status := model.AssetVersionStatus(strings.TrimSpace(payload.Status))
	if status == "" {
		status = model.AssetVersionStatusConfirmed
	}
	return model.Asset{
		ID:               id,
		UserID:           userID,
		Kind:             strings.TrimSpace(payload.Kind),
		Category:         category,
		Status:           status,
		PrimaryVersionID: primaryVersionID,
		Title:            strings.TrimSpace(payload.Title),
		PayloadJSON:      string(raw),
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}, nil
}

func canvasProjectFromJSON(userID string, raw json.RawMessage) (model.CanvasProject, error) {
	if err := validateSyncedPayload(raw, "画布"); err != nil {
		return model.CanvasProject{}, err
	}
	var payload struct {
		ID            string `json:"id"`
		Title         string `json:"title"`
		ProjectID     string `json:"projectId"`
		AigcProjectID *int64 `json:"aigcProjectId"`
		CreatedAt     string `json:"createdAt"`
		UpdatedAt     string `json:"updatedAt"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return model.CanvasProject{}, BadAuthRequest("画布数据格式错误")
	}
	now := time.Now()
	createdAt := parseClientTime(payload.CreatedAt, now)
	updatedAt := parseClientTime(payload.UpdatedAt, createdAt)
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		id = newID()
	}
	return model.CanvasProject{
		ID:            id,
		UserID:        userID,
		ProjectID:     strings.TrimSpace(payload.ProjectID),
		AigcProjectID: normalizeOptionalInt64(payload.AigcProjectID),
		Title:         strings.TrimSpace(payload.Title),
		PayloadJSON:   string(raw),
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

func (s *Service) inheritCanvasAigcProject(userID string, project *model.CanvasProject) error {
	if project.AigcProjectID != nil {
		returnValue, err := s.ValidateAigcProjectForUser(userID, project.AigcProjectID)
		if err != nil {
			return err
		}
		project.AigcProjectID = returnValue
		return nil
	}
	if strings.TrimSpace(project.ProjectID) == "" {
		project.AigcProjectID = nil
		return nil
	}
	domainProject, err := s.repo.ProjectForUser(userID, project.ProjectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return BadAuthRequest("画布关联的短剧项目不存在或无权访问")
		}
		return err
	}
	project.AigcProjectID = domainProject.AigcProjectID
	return nil
}

func normalizeOptionalInt64(value *int64) *int64 {
	if value == nil || *value <= 0 {
		return nil
	}
	next := *value
	return &next
}

func validateSyncedPayload(raw json.RawMessage, label string) error {
	if len(raw) > 4<<20 {
		return BadAuthRequest(label + "数据超过 4MB，请先把媒体文件保存到资源存储")
	}
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err == nil && containsInlineMediaDataURL(payload) {
		return BadAuthRequest(label + "数据包含内嵌媒体，请先上传到资源存储")
	}
	return nil
}

// 同步数据只禁止作为字段值存在的媒体 Data URL；提示词和上游错误文案可能合法提到相同字符串。
func containsInlineMediaDataURL(value interface{}) bool {
	switch item := value.(type) {
	case string:
		text := strings.ToLower(strings.TrimSpace(item))
		return strings.HasPrefix(text, "data:image/") || strings.HasPrefix(text, "data:video/") || strings.HasPrefix(text, "data:audio/")
	case []interface{}:
		for _, child := range item {
			if containsInlineMediaDataURL(child) {
				return true
			}
		}
	case map[string]interface{}:
		for _, child := range item {
			if containsInlineMediaDataURL(child) {
				return true
			}
		}
	}
	return false
}

func parseClientTime(value string, fallback time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	return fallback
}
