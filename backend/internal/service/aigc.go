package service

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

type AigcDepartmentRequest struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Remark string `json:"remark"`
}

type AigcProjectRequest struct {
	ParentID          *int64 `json:"parentId"`
	DeptID            *int64 `json:"deptId"`
	FirstCategoryID   *int64 `json:"firstCategoryId"`
	FirstCategoryName string `json:"firstCategoryName"`
	ProjectName       string `json:"projectName"`
	ProjectNameOuter  string `json:"projectNameOuter"`
	ProjectType       string `json:"projectType"`
	ProjectDesc       string `json:"projectDesc"`
	Status            string `json:"status"`
	Level             int    `json:"level"`
	Remark            string `json:"remark"`
}

type AigcProjectPage struct {
	Projects []model.AigcProject `json:"projects"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	Limit    int                 `json:"limit"`
}

type AigcProjectTreeNode struct {
	Project  model.AigcProject      `json:"project"`
	Children []AigcProjectTreeNode `json:"children,omitempty"`
}

func (s *Service) RequireAigcProjectManager(user *model.User) error {
	if user == nil {
		return Unauthorized("请先登录")
	}
	if user.Status != model.UserStatusActive {
		return Forbidden("当前账号已停用")
	}
	if user.Role != model.UserRoleAdmin && user.Role != model.UserRoleTeamLead {
		return Forbidden("需要团队主管权限")
	}
	return nil
}

func (s *Service) AigcDepartments(actor *model.User, keyword string) ([]model.AigcDepartment, error) {
	if err := s.RequireAigcProjectManager(actor); err != nil {
		return nil, err
	}
	if actor.Role == model.UserRoleTeamLead {
		if actor.DeptID == nil {
			return nil, Forbidden("团队主管未设置团队")
		}
		department, err := s.repo.AigcDepartment(*actor.DeptID)
		if err != nil {
			return nil, err
		}
		if value := strings.ToLower(strings.TrimSpace(keyword)); value != "" && !strings.Contains(strings.ToLower(department.Name), value) {
			return []model.AigcDepartment{}, nil
		}
		return []model.AigcDepartment{*department}, nil
	}
	return s.repo.AigcDepartments(keyword)
}

func (s *Service) CreateAigcDepartment(actor *model.User, req AigcDepartmentRequest) (*model.AigcDepartment, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, BadAuthRequest("请填写团队名称")
	}
	status, err := normalizeAigcTeamStatus(req.Status)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	department := &model.AigcDepartment{ParentID: 0, Name: name, Status: status, Remark: truncateRunes(strings.TrimSpace(req.Remark), 500), CreatedBy: actor.ID, UpdatedBy: actor.ID, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.Create(department); err != nil {
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "aigc.team.create", "aigc_team", formatIntID(department.DeptID), "创建团队", map[string]any{"name": department.Name}); err != nil {
		return nil, err
	}
	return department, nil
}

func (s *Service) UpdateAigcDepartment(actor *model.User, id string, req AigcDepartmentRequest) (*model.AigcDepartment, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	departmentID, err := parseRequiredIntID(id, "团队 ID 无效")
	if err != nil {
		return nil, err
	}
	department, err := s.repo.AigcDepartment(departmentID)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, BadAuthRequest("请填写团队名称")
	}
	status, err := normalizeAigcTeamStatus(req.Status)
	if err != nil {
		return nil, err
	}
	department.ParentID = 0
	department.Name = name
	department.Status = status
	department.Remark = truncateRunes(strings.TrimSpace(req.Remark), 500)
	department.UpdatedBy = actor.ID
	department.UpdatedAt = time.Now()
	if err := s.repo.Save(department); err != nil {
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "aigc.team.update", "aigc_team", formatIntID(department.DeptID), "更新团队", map[string]any{"name": department.Name}); err != nil {
		return nil, err
	}
	return department, nil
}

func normalizeAigcTeamStatus(value string) (string, error) {
	status := strings.TrimSpace(value)
	if status == "" {
		return "启用", nil
	}
	if status != "启用" && status != "禁用" {
		return "", BadAuthRequest("团队状态必须为启用或禁用")
	}
	return status, nil
}

func (s *Service) AigcProjects(actor *model.User, query AdminListQuery, level string, parentID string) (*AigcProjectPage, error) {
	if err := s.RequireAigcProjectManager(actor); err != nil {
		return nil, err
	}
	page, limit := normalizeAdminPage(query.Page, query.Limit)
	parsedParentID, err := parseOptionalIntID(parentID, "上级项目 ID 无效")
	if err != nil {
		return nil, err
	}
	var visibleDeptID *int64
	if actor.Role == model.UserRoleTeamLead {
		if actor.DeptID == nil {
			return nil, Forbidden("团队主管未设置团队")
		}
		visibleDeptID = actor.DeptID
	}
	projects, total, err := s.repo.AigcProjects(query.Keyword, query.Status, level, parsedParentID, visibleDeptID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	return &AigcProjectPage{Projects: projects, Total: total, Page: page, Limit: limit}, nil
}

func (s *Service) AvailableAigcSecondLevelProjects(actor *model.User) ([]model.AigcProject, error) {
	if actor == nil {
		return nil, Unauthorized("请先登录")
	}
	if actor.Status != model.UserStatusActive {
		return nil, Forbidden("当前账号已停用")
	}
	if actor.Role == model.UserRoleAdmin {
		return s.repo.AigcSecondLevelProjects()
	}
	if actor.DeptID == nil {
		return []model.AigcProject{}, nil
	}
	return s.repo.AigcSecondLevelProjectsByDept(*actor.DeptID)
}

func (s *Service) AvailableAigcProjectTree(actor *model.User) ([]AigcProjectTreeNode, error) {
	if actor == nil {
		return nil, Unauthorized("请先登录")
	}
	if actor.Status != model.UserStatusActive {
		return nil, Forbidden("当前账号已停用")
	}
	roots, err := s.repo.AigcAvailableFirstLevelProjects()
	if err != nil {
		return nil, err
	}
	var secondLevel []model.AigcProject
	switch actor.Role {
	case model.UserRoleAdmin:
		secondLevel, err = s.repo.AigcAvailableSecondLevelProjects(nil)
	case model.UserRoleTeamLead:
		if actor.DeptID == nil {
			return buildAigcProjectTree(roots, nil), nil
		}
		secondLevel, err = s.repo.AigcAvailableSecondLevelProjects(actor.DeptID)
	default:
		if actor.DeptID == nil {
			return buildAigcProjectTree(roots, nil), nil
		}
		secondLevel, err = s.repo.AigcAvailableSecondLevelProjects(actor.DeptID)
	}
	if err != nil {
		return nil, err
	}
	return buildAigcProjectTree(roots, secondLevel), nil
}

func (s *Service) CreateAigcProject(actor *model.User, req AigcProjectRequest) (*model.AigcProject, error) {
	if err := s.RequireAigcProjectManager(actor); err != nil {
		return nil, err
	}
	project, err := s.aigcProjectFromRequest(req, model.AigcProject{CreatedBy: actor.ID, UpdatedBy: actor.ID, CreatedAt: time.Now()})
	if err != nil {
		return nil, err
	}
	if err := requireAigcFirstLevelAdmin(actor, project.Level); err != nil {
		return nil, err
	}
	if err := s.applyAigcProjectDepartment(actor, &project, false); err != nil {
		return nil, err
	}
	if err := s.ensureAigcProjectNameUnique(project.ProjectName, 0); err != nil {
		return nil, err
	}
	if project.Level == 2 && project.DeptID <= 0 {
		return nil, BadAuthRequest("二级项目必须选择团队")
	}
	project.UpdatedAt = project.CreatedAt
	if err := s.repo.Create(&project); err != nil {
		return nil, err
	}
	if project.Level == 1 && project.FirstCategoryID == nil {
		project.FirstCategoryID = &project.ProjectID
		if err := s.repo.Save(&project); err != nil {
			return nil, err
		}
	}
	if err := s.appendAdminAudit(actor, "aigc.project.create", "aigc_project", formatIntID(project.ProjectID), "创建运营项目", map[string]any{"name": project.ProjectName}); err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Service) UpdateAigcProject(actor *model.User, id string, req AigcProjectRequest) (*model.AigcProject, error) {
	if err := s.RequireAigcProjectManager(actor); err != nil {
		return nil, err
	}
	projectID, err := parseRequiredIntID(id, "项目 ID 无效")
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.AigcProject(projectID)
	if err != nil {
		return nil, err
	}
	if err := s.requireAigcProjectWritable(actor, existing); err != nil {
		return nil, err
	}
	project, err := s.aigcProjectFromRequest(req, *existing)
	if err != nil {
		return nil, err
	}
	if err := requireAigcFirstLevelAdmin(actor, existing.Level); err != nil {
		return nil, err
	}
	if err := requireAigcFirstLevelAdmin(actor, project.Level); err != nil {
		return nil, err
	}
	allowDisabledTeam := existing.Level == 2 && project.Level == 2 && existing.DeptID == project.DeptID
	if err := s.applyAigcProjectDepartment(actor, &project, allowDisabledTeam); err != nil {
		return nil, err
	}
	if err := s.ensureAigcProjectNameUnique(project.ProjectName, project.ProjectID); err != nil {
		return nil, err
	}
	project.UpdatedBy = actor.ID
	project.UpdatedAt = time.Now()
	if err := s.repo.Save(&project); err != nil {
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "aigc.project.update", "aigc_project", formatIntID(project.ProjectID), "更新运营项目", map[string]any{"name": project.ProjectName}); err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Service) DeleteAigcProject(actor *model.User, id string) error {
	if err := s.RequireAigcProjectManager(actor); err != nil {
		return err
	}
	if _, err := parseRequiredIntID(id, "项目 ID 无效"); err != nil {
		return err
	}
	return BadAuthRequest("项目不支持删除，请改为禁用")
}

func (s *Service) validateTaskAigcProject(userID string, id *int64) (*int64, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	if *id < 0 {
		return nil, BadAuthRequest("运营项目 ID 无效")
	}
	user, err := s.repo.User(userID)
	if err != nil {
		return nil, err
	}
	if user.Status != model.UserStatusActive {
		return nil, Forbidden("当前账号已停用")
	}
	project, err := s.repo.AigcProject(*id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, BadAuthRequest("所选运营项目不存在")
	}
	if err != nil {
		return nil, err
	}
	if project.Status != "启用" {
		return nil, BadAuthRequest("所选运营项目已禁用")
	}
	if project.Level == 2 && user.Role != model.UserRoleAdmin {
		if user.DeptID == nil || project.DeptID != *user.DeptID {
			return nil, Forbidden("无权使用所选运营项目")
		}
	}
	value := *id
	return &value, nil
}

// ValidateAigcProjectForUser 校验前端透传的项目归属，系统代理计费不能直接信任请求头。
func (s *Service) ValidateAigcProjectForUser(userID string, id *int64) (*int64, error) {
	return s.validateTaskAigcProject(userID, id)
}

func (s *Service) ValidateAigcAssignableDepartment(id *int64, allowRoot bool) error {
	return s.validateAigcAssignableDepartment(id, false)
}

func (s *Service) validateAigcAssignableDepartment(id *int64, allowDisabled bool) error {
	if id == nil {
		return nil
	}
	department, err := s.repo.AigcDepartment(*id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return BadAuthRequest("所选团队不存在")
	}
	if err != nil {
		return err
	}
	if department.Status != "启用" && !allowDisabled {
		return BadAuthRequest("所选团队已禁用")
	}
	return nil
}

func (s *Service) aigcProjectFromRequest(req AigcProjectRequest, project model.AigcProject) (model.AigcProject, error) {
	project.ProjectName = strings.TrimSpace(req.ProjectName)
	project.ProjectNameOuter = strings.TrimSpace(req.ProjectNameOuter)
	project.ProjectType = strings.TrimSpace(req.ProjectType)
	if project.ProjectType == "" {
		project.ProjectType = "默认"
	}
	project.FirstCategoryID = req.FirstCategoryID
	project.FirstCategoryName = strings.TrimSpace(req.FirstCategoryName)
	project.ProjectDesc = strings.TrimSpace(req.ProjectDesc)
	project.Status = strings.TrimSpace(req.Status)
	project.Level = req.Level
	project.ParentID = int64Value(req.ParentID)
	project.DeptID = int64Value(req.DeptID)
	project.Remark = truncateRunes(strings.TrimSpace(req.Remark), 500)
	if project.ProjectName == "" || project.ProjectType == "" {
		return project, BadAuthRequest("请填写项目名称和项目类型")
	}
	if project.Status != "启用" && project.Status != "禁用" {
		return project, BadAuthRequest("项目状态必须为启用或禁用")
	}
	if project.Level != 1 && project.Level != 2 {
		return project, BadAuthRequest("项目级别必须为 1 或 2")
	}
	if project.Level == 1 {
		project.ParentID = 0
		project.DeptID = 0
		if project.FirstCategoryID == nil {
			if project.ProjectID > 0 {
				id := project.ProjectID
				project.FirstCategoryID = &id
			}
		}
		if project.FirstCategoryName == "" {
			project.FirstCategoryName = project.ProjectName
		}
		return project, nil
	}
	if project.ParentID <= 0 {
		return project, BadAuthRequest("二级项目必须选择一级项目")
	}
	parent, err := s.repo.AigcProject(project.ParentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return project, BadAuthRequest("上级项目不存在")
	}
	if err != nil {
		return project, err
	}
	if parent.Level != 1 {
		return project, BadAuthRequest("二级项目只能归属一级项目")
	}
	project.FirstCategoryID = &parent.ProjectID
	project.FirstCategoryName = parent.ProjectName
	return project, nil
}

func buildAigcProjectTree(roots []model.AigcProject, secondLevel []model.AigcProject) []AigcProjectTreeNode {
	childrenByParent := make(map[int64][]AigcProjectTreeNode, len(roots))
	for _, project := range secondLevel {
		parentID := project.ParentID
		childrenByParent[parentID] = append(childrenByParent[parentID], AigcProjectTreeNode{Project: project})
	}
	nodes := make([]AigcProjectTreeNode, 0, len(roots))
	for _, root := range roots {
		node := AigcProjectTreeNode{Project: root, Children: childrenByParent[root.ProjectID]}
		nodes = append(nodes, node)
	}
	return nodes
}

func (s *Service) ensureAigcProjectNameUnique(name string, currentID int64) error {
	existing, err := s.repo.AigcProjectByName(name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.ProjectID != currentID {
		return BadAuthRequest("运营项目名称不能重复")
	}
	return nil
}

func requireAigcFirstLevelAdmin(actor *model.User, level int) error {
	if level == 1 && actor.Role != model.UserRoleAdmin {
		return Forbidden("只有管理员可以创建或编辑一级项目")
	}
	return nil
}

func (s *Service) applyAigcProjectDepartment(actor *model.User, project *model.AigcProject, allowDisabled bool) error {
	if project.Level == 1 {
		project.ParentID = 0
		project.DeptID = 0
		return nil
	}
	if actor.Role == model.UserRoleTeamLead {
		if actor.DeptID == nil {
			return BadAuthRequest("团队主管未设置团队")
		}
		project.DeptID = *actor.DeptID
	}
	if project.DeptID <= 0 {
		return BadAuthRequest("二级项目必须选择团队")
	}
	return s.validateAigcAssignableDepartment(&project.DeptID, allowDisabled)
}

func (s *Service) requireAigcProjectWritable(actor *model.User, project *model.AigcProject) error {
	if actor.Role == model.UserRoleAdmin {
		return nil
	}
	if project.Level != 2 || project.DeptID <= 0 || actor.DeptID == nil || project.DeptID != *actor.DeptID {
		return Forbidden("只能修改本团队的二级项目")
	}
	return nil
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func parseRequiredIntID(value string, message string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, BadAuthRequest(message)
	}
	return id, nil
}

func parseOptionalIntID(value string, message string) (*int64, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	id, err := parseRequiredIntID(value, message)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func formatIntID(id int64) string {
	return strconv.FormatInt(id, 10)
}
