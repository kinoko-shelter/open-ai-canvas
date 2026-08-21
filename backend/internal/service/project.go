package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

type CreateProjectRequest struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	AigcProjectID    *int64 `json:"aigcProjectId"`
	AspectRatio      string `json:"aspectRatio"`
	SourceType       string `json:"sourceType"`
	Description      string `json:"description"`
	StylePresetID    string `json:"stylePresetId"`
	StyleProfileJSON string `json:"styleProfileJson"`
}

type UpdateProjectRequest struct {
	Name             string  `json:"name"`
	Type             string  `json:"type"`
	AigcProjectID    *int64  `json:"aigcProjectId"`
	AspectRatio      string  `json:"aspectRatio"`
	SourceType       string  `json:"sourceType"`
	Description      *string `json:"description"`
	StylePresetID    *string `json:"stylePresetId"`
	StyleProfileJSON *string `json:"styleProfileJson"`
	Status           string  `json:"status"`
}

type CreateProjectUnitRequest struct {
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	SourceText string `json:"sourceText"`
	Position   int    `json:"position"`
}

type UpdateProjectUnitRequest struct {
	Title      string `json:"title"`
	SourceText string `json:"sourceText"`
	Status     string `json:"status"`
}

type ImportProjectUnitsRequest struct {
	Units []CreateProjectUnitRequest `json:"units"`
}

type ReorderProjectUnitsRequest struct {
	UnitIDs []string `json:"unitIds"`
}

type LinkCanvasUnitRequest struct {
	CanvasID string `json:"canvasId"`
	UnitID   string `json:"unitId"`
	Role     string `json:"role"`
}

type ProjectSummary struct {
	Project            model.Project `json:"project"`
	CanvasCount        int           `json:"canvasCount"`
	AssetCount         int64         `json:"assetCount"`
	UnitCount          int           `json:"unitCount"`
	CompletedUnitCount int           `json:"completedUnitCount"`
	CreatorName        string        `json:"creatorName,omitempty"`
	CreatorUsername    string        `json:"creatorUsername,omitempty"`
	DeptID             *int64        `json:"deptId,omitempty"`
	DeptName           string        `json:"deptName,omitempty"`
}

type ProjectDetail struct {
	Project         model.Project                 `json:"project"`
	Units           []model.ProjectUnit           `json:"units"`
	Canvases        []model.CanvasProject         `json:"canvases"`
	CanvasUnitLinks []model.CanvasUnitLink        `json:"canvasUnitLinks"`
	Assets          []ProjectAssetSummary         `json:"assets"`
	Workflows       []ProjectWorkflowDetail       `json:"workflows"`
	Shots           []model.Shot                  `json:"shots"`
	ShotReferences  []model.ShotAssetReference    `json:"shotReferences"`
	AssetCandidates []model.ProjectAssetCandidate `json:"assetCandidates"`
}

func (s *Service) ListProjects(userID string) ([]ProjectSummary, error) {
	scope, err := s.dataScopeForUserID(userID)
	if err != nil {
		return nil, err
	}
	projects, err := s.repo.ProjectsForScope(scope)
	if err != nil {
		return nil, err
	}
	result := make([]ProjectSummary, 0, len(projects))
	owners, err := s.repo.UsersByIDs(projectOwnerIDs(projects))
	if err != nil {
		return nil, err
	}
	departmentNames, err := s.repo.AigcDepartmentNames(userDeptIDs(owners))
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		units, unitsErr := s.repo.ProjectUnitSummaries(project.ID)
		if unitsErr != nil {
			return nil, unitsErr
		}
		canvases, canvasesErr := s.repo.ProjectCanvasSummariesForScope(scope, project.ID)
		if canvasesErr != nil {
			return nil, canvasesErr
		}
		assetCount, assetCountErr := s.repo.ProjectAssetCount(project.ID)
		if assetCountErr != nil {
			return nil, assetCountErr
		}
		completed := 0
		for _, unit := range units {
			if unit.Status == model.ProjectUnitStatusCompleted {
				completed++
			}
		}
		summary := ProjectSummary{Project: project, CanvasCount: len(canvases), AssetCount: assetCount, UnitCount: len(units), CompletedUnitCount: completed}
		if owner, ok := owners[project.UserID]; ok {
			summary.CreatorName = normalizeDisplayName(owner.DisplayName, owner.Username)
			summary.CreatorUsername = owner.Username
			if owner.DeptID != nil {
				deptID := *owner.DeptID
				summary.DeptID = &deptID
				summary.DeptName = departmentNames[deptID]
			}
		}
		result = append(result, summary)
	}
	return result, nil
}

func (s *Service) ProjectDetail(userID string, id string) (ProjectDetail, error) {
	project, err := s.projectForUserID(userID, id)
	if err != nil {
		return ProjectDetail{}, err
	}
	// 项目读取允许降级修复：旧任务可能已成功持久化图片，但浏览器刷新中断了角色版本绑定。
	if s.reconcileCharacterTurnaroundTasks(userID, project.ID) {
		project, err = s.projectForUserID(userID, id)
		if err != nil {
			return ProjectDetail{}, err
		}
	}
	// 项目工作台只返回章节摘要，长篇小说正文由单章接口按需读取。
	units, err := s.repo.ProjectUnitSummaries(project.ID)
	if err != nil {
		return ProjectDetail{}, err
	}
	scope, err := s.dataScopeForUserID(userID)
	if err != nil {
		return ProjectDetail{}, err
	}
	canvases, err := s.repo.ProjectCanvasSummariesForScope(scope, project.ID)
	if err != nil {
		return ProjectDetail{}, err
	}
	canvasUnitLinks, err := s.repo.ProjectCanvasUnitLinks(project.ID)
	if err != nil {
		return ProjectDetail{}, err
	}
	assets, err := s.ProjectAssets(userID, project.ID)
	if err != nil {
		return ProjectDetail{}, err
	}
	workflows, err := s.ProjectWorkflows(project.ID)
	if err != nil {
		return ProjectDetail{}, err
	}
	shots, err := s.repo.ProjectShots(project.ID)
	if err != nil {
		return ProjectDetail{}, err
	}
	shotReferences, err := s.repo.ProjectShotAssetReferences(project.ID)
	if err != nil {
		return ProjectDetail{}, err
	}
	candidates, err := s.repo.ProjectAssetCandidates(project.ID)
	if err != nil {
		return ProjectDetail{}, err
	}
	return ProjectDetail{Project: *project, Units: units, Canvases: canvases, CanvasUnitLinks: canvasUnitLinks, Assets: assets, Workflows: workflows, Shots: shots, ShotReferences: shotReferences, AssetCandidates: candidates}, nil
}

func (s *Service) CreateProject(userID string, req CreateProjectRequest) (model.Project, error) {
	if err := s.EnsureBuiltinProjectWorkflowTemplate(); err != nil {
		return model.Project{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.Project{}, BadAuthRequest("项目名称不能为空")
	}
	projectType := strings.TrimSpace(req.Type)
	if projectType == "" {
		projectType = "short-drama"
	}
	aspectRatio := strings.TrimSpace(req.AspectRatio)
	if aspectRatio == "" {
		aspectRatio = "9:16"
	}
	sourceType := strings.TrimSpace(req.SourceType)
	if sourceType == "" {
		sourceType = "blank"
	}
	styleProfileJSON, err := validateStyleProfileJSON(req.StyleProfileJSON)
	if err != nil {
		return model.Project{}, BadAuthRequest(err.Error())
	}
	stylePresetID := strings.TrimSpace(req.StylePresetID)
	if err := validateStyleProfilePreset(stylePresetID, styleProfileJSON); err != nil {
		return model.Project{}, BadAuthRequest(err.Error())
	}
	aigcProjectID, err := s.ValidateAigcProjectForUser(userID, req.AigcProjectID)
	if err != nil {
		return model.Project{}, err
	}
	if aigcProjectID == nil {
		return model.Project{}, BadAuthRequest("请选择业务项目")
	}
	now := time.Now()
	project := model.Project{ID: newID(), UserID: userID, Name: name, Type: projectType, AigcProjectID: aigcProjectID, AspectRatio: aspectRatio, SourceType: sourceType, Description: strings.TrimSpace(req.Description), StylePresetID: stylePresetID, StyleProfileJSON: styleProfileJSON, Status: model.ProjectStatusActive, Revision: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateProject(&project); err != nil {
		return model.Project{}, err
	}
	if _, err := s.createProjectWorkflow(project.ID, "", "project"); err != nil {
		_ = s.repo.DeleteProject(userID, project.ID)
		return model.Project{}, err
	}
	project.Revision++
	project.UpdatedAt = time.Now()
	return project, nil
}

func (s *Service) UpdateProject(userID string, id string, req UpdateProjectRequest) (model.Project, error) {
	project, err := s.projectForUserID(userID, id)
	if err != nil {
		return model.Project{}, err
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		project.Name = name
	}
	if value := strings.TrimSpace(req.Type); value != "" {
		project.Type = value
	}
	if value := strings.TrimSpace(req.AspectRatio); value != "" {
		project.AspectRatio = value
	}
	if value := strings.TrimSpace(req.SourceType); value != "" {
		project.SourceType = value
	}
	if req.AigcProjectID != nil {
		aigcProjectID, aigcProjectErr := s.ValidateAigcProjectForUser(userID, req.AigcProjectID)
		if aigcProjectErr != nil {
			return model.Project{}, aigcProjectErr
		}
		project.AigcProjectID = aigcProjectID
	}
	if req.Description != nil {
		project.Description = strings.TrimSpace(*req.Description)
	}
	if req.StylePresetID != nil {
		project.StylePresetID = strings.TrimSpace(*req.StylePresetID)
	}
	if req.StyleProfileJSON != nil {
		styleProfileJSON, profileErr := validateStyleProfileJSON(*req.StyleProfileJSON)
		if profileErr != nil {
			return model.Project{}, BadAuthRequest(profileErr.Error())
		}
		project.StyleProfileJSON = styleProfileJSON
	}
	if err := validateStyleProfilePreset(project.StylePresetID, project.StyleProfileJSON); err != nil {
		return model.Project{}, BadAuthRequest(err.Error())
	}
	if status := model.ProjectStatus(strings.TrimSpace(req.Status)); status != "" {
		if status != model.ProjectStatusActive && status != model.ProjectStatusArchived {
			return model.Project{}, BadAuthRequest("不支持的项目状态")
		}
		project.Status = status
	}
	project.Revision++
	project.UpdatedAt = time.Now()
	if err := s.repo.UpdateProject(project); err != nil {
		return model.Project{}, err
	}
	return *project, nil
}

func (s *Service) DeleteProject(userID string, id string) error {
	project, err := s.projectForUserID(userID, id)
	if err != nil {
		return err
	}
	return s.repo.DeleteProject(project.UserID, id)
}

func (s *Service) CreateProjectUnit(userID string, projectID string, req CreateProjectUnitRequest) (model.ProjectUnit, error) {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return model.ProjectUnit{}, err
	}
	unit, err := newProjectUnit(projectID, req, req.Position)
	if err != nil {
		return model.ProjectUnit{}, err
	}
	if err := s.repo.CreateProjectUnit(&unit); err != nil {
		return model.ProjectUnit{}, err
	}
	if err := s.repo.BumpProjectRevision(projectID); err != nil {
		return model.ProjectUnit{}, err
	}
	return unit, nil
}

func (s *Service) GetProjectUnit(userID string, projectID string, unitID string) (model.ProjectUnit, error) {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return model.ProjectUnit{}, err
	}
	unit, err := s.repo.ProjectUnit(projectID, strings.TrimSpace(unitID))
	if err != nil {
		return model.ProjectUnit{}, err
	}
	return *unit, nil
}

func (s *Service) ImportProjectUnits(userID string, projectID string, req ImportProjectUnitsRequest) ([]model.ProjectUnit, error) {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return nil, err
	}
	if len(req.Units) == 0 || len(req.Units) > 2500 {
		return nil, BadAuthRequest("一次导入的章节数量必须在 1 到 2500 之间")
	}
	existing, err := s.repo.ProjectUnits(projectID)
	if err != nil {
		return nil, err
	}
	units := make([]model.ProjectUnit, 0, len(req.Units))
	for index, input := range req.Units {
		unit, unitErr := newProjectUnit(projectID, input, len(existing)+index)
		if unitErr != nil {
			return nil, unitErr
		}
		units = append(units, unit)
	}
	if err := s.repo.ImportProjectUnits(projectID, units); err != nil {
		return nil, err
	}
	return units, nil
}

func (s *Service) ReorderProjectUnits(userID string, projectID string, req ReorderProjectUnitsRequest) error {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return err
	}
	units, err := s.repo.ProjectUnits(projectID)
	if err != nil {
		return err
	}
	if len(req.UnitIDs) != len(units) {
		return BadAuthRequest("章节排序列表不完整")
	}
	existing := make(map[string]struct{}, len(units))
	for _, unit := range units {
		existing[unit.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(req.UnitIDs))
	normalizedIDs := make([]string, 0, len(req.UnitIDs))
	for _, rawID := range req.UnitIDs {
		id := strings.TrimSpace(rawID)
		if _, ok := existing[id]; !ok {
			return BadAuthRequest("章节排序包含无效章节")
		}
		if _, duplicate := seen[id]; duplicate {
			return BadAuthRequest("章节排序包含重复章节")
		}
		seen[id] = struct{}{}
		normalizedIDs = append(normalizedIDs, id)
	}
	return s.repo.ReorderProjectUnits(projectID, normalizedIDs)
}

func (s *Service) DeleteProjectUnit(userID string, projectID string, unitID string) error {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return err
	}
	if _, err := s.repo.ProjectUnit(projectID, unitID); err != nil {
		return err
	}
	return s.repo.DeleteProjectUnit(projectID, unitID)
}

func newProjectUnit(projectID string, req CreateProjectUnitRequest, position int) (model.ProjectUnit, error) {
	kind := model.ProjectUnitKind(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = model.ProjectUnitKindChapter
	}
	if kind != model.ProjectUnitKindChapter && kind != model.ProjectUnitKindEpisode {
		return model.ProjectUnit{}, BadAuthRequest("不支持的项目单元类型")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return model.ProjectUnit{}, BadAuthRequest("章节标题不能为空")
	}
	if position < 0 {
		position = 0
	}
	now := time.Now()
	unit := model.ProjectUnit{ID: newID(), ProjectID: projectID, Kind: kind, Title: title, SourceText: req.SourceText, Status: model.ProjectUnitStatusDraft, Position: position, CreatedAt: now, UpdatedAt: now}
	return unit, nil
}

func (s *Service) UpdateProjectUnit(userID string, projectID string, unitID string, req UpdateProjectUnitRequest) (model.ProjectUnit, error) {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return model.ProjectUnit{}, err
	}
	unit, err := s.repo.ProjectUnit(projectID, unitID)
	if err != nil {
		return model.ProjectUnit{}, err
	}
	if title := strings.TrimSpace(req.Title); title != "" {
		unit.Title = title
	}
	unit.SourceText = req.SourceText
	if status := model.ProjectUnitStatus(strings.TrimSpace(req.Status)); status != "" {
		if status != model.ProjectUnitStatusDraft && status != model.ProjectUnitStatusReady && status != model.ProjectUnitStatusCompleted {
			return model.ProjectUnit{}, BadAuthRequest("不支持的章节状态")
		}
		unit.Status = status
	}
	unit.UpdatedAt = time.Now()
	if err := s.repo.UpdateProjectUnit(unit); err != nil {
		return model.ProjectUnit{}, err
	}
	if err := s.repo.BumpProjectRevision(projectID); err != nil {
		return model.ProjectUnit{}, err
	}
	return *unit, nil
}

func (s *Service) LinkCanvasUnit(userID string, projectID string, req LinkCanvasUnitRequest) (model.CanvasUnitLink, error) {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return model.CanvasUnitLink{}, err
	}
	canvasID := strings.TrimSpace(req.CanvasID)
	unitID := strings.TrimSpace(req.UnitID)
	if canvasID == "" || unitID == "" {
		return model.CanvasUnitLink{}, BadAuthRequest("画布和章节不能为空")
	}
	canvas, err := s.canvasProjectForUserID(userID, canvasID)
	if err != nil {
		return model.CanvasUnitLink{}, err
	}
	if _, err := s.repo.ProjectUnit(projectID, unitID); err != nil {
		return model.CanvasUnitLink{}, err
	}
	if err := s.repo.AssignCanvasToProject(canvas.UserID, canvasID, projectID); err != nil {
		return model.CanvasUnitLink{}, err
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "storyboard"
	}
	now := time.Now()
	link := model.CanvasUnitLink{ID: newID(), ProjectID: projectID, CanvasID: canvasID, UnitID: unitID, Role: role, CreatedAt: now}
	if err := s.repo.UpsertCanvasUnitLink(&link); err != nil {
		return model.CanvasUnitLink{}, err
	}
	if err := s.repo.BumpProjectRevision(projectID); err != nil {
		return model.CanvasUnitLink{}, err
	}
	return link, nil
}

func (s *Service) UnlinkCanvasUnit(userID string, projectID string, canvasID string, unitID string) error {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return err
	}
	canvas, err := s.canvasProjectForUserID(userID, strings.TrimSpace(canvasID))
	if err != nil {
		return err
	}
	if canvas.ProjectID != projectID {
		return BadAuthRequest("画布不属于当前项目")
	}
	if _, err := s.repo.CanvasUnitLink(projectID, canvas.ID, strings.TrimSpace(unitID)); err != nil {
		return err
	}
	return s.repo.DeleteCanvasUnitLink(projectID, canvas.ID, strings.TrimSpace(unitID))
}

func (s *Service) UnlinkCanvasProject(userID string, projectID string, canvasID string) error {
	if _, err := s.projectForUserID(userID, projectID); err != nil {
		return err
	}
	canvas, err := s.canvasProjectForUserID(userID, strings.TrimSpace(canvasID))
	if err != nil {
		return err
	}
	if canvas.ProjectID != projectID {
		return BadAuthRequest("画布不属于当前项目")
	}
	now := time.Now()
	payloadJSON, err := canvasPayloadWithoutProject(canvas.PayloadJSON, now)
	if err != nil {
		return err
	}
	// 关系列、同步快照和更新时间必须原子更新，否则浏览器会用旧 projectId 把关系重新写回。
	return s.repo.UnassignCanvasFromProject(canvas.UserID, projectID, canvas.ID, payloadJSON, now)
}

func canvasPayloadWithoutProject(payloadJSON string, updatedAt time.Time) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return "", BadAuthRequest("画布数据格式错误，无法解除项目关系")
	}
	delete(payload, "projectId")
	payload["updatedAt"] = updatedAt.Format(time.RFC3339Nano)
	next, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(next), nil
}

func projectOwnerIDs(projects []model.Project) []string {
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.UserID)
	}
	return uniqueNonEmptyStrings(ids)
}

func userDeptIDs(users map[string]model.User) []int64 {
	seen := make(map[int64]struct{}, len(users))
	ids := make([]int64, 0, len(users))
	for _, user := range users {
		if user.DeptID == nil {
			continue
		}
		if _, ok := seen[*user.DeptID]; ok {
			continue
		}
		seen[*user.DeptID] = struct{}{}
		ids = append(ids, *user.DeptID)
	}
	return ids
}

func IsProjectNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// 任务仍以画布 ID 作为 projectId；写入前必须解析到业务项目并阻止归档项目继续生成。
func (s *Service) ensureTaskProjectActive(userID string, canvasOrProjectID string) error {
	actor, err := s.actorForUserID(userID)
	if err != nil {
		return err
	}
	return s.ensureTaskProjectActiveForUser(actor, canvasOrProjectID)
}

func (s *Service) ensureTaskProjectActiveForUser(actor *model.User, canvasOrProjectID string) error {
	id := strings.TrimSpace(canvasOrProjectID)
	if id == "" {
		return nil
	}
	if canvas, err := s.scopedCanvasProject(actor, id); err == nil {
		if canvas.ProjectID == "" {
			return nil
		}
		project, projectErr := s.scopedProject(actor, canvas.ProjectID)
		if projectErr != nil {
			return projectErr
		}
		if project.Status == model.ProjectStatusArchived {
			return BadAuthRequest("项目已归档，无法创建生成任务")
		}
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	project, err := s.scopedProject(actor, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if project.Status == model.ProjectStatusArchived {
		return BadAuthRequest("项目已归档，无法创建生成任务")
	}
	return nil
}
