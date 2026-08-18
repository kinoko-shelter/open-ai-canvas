package repository

import (
	"strconv"
	"strings"

	"infinite-canvas/backend/internal/model"
)

func (r *Repository) AigcDepartments(keyword string) ([]model.AigcDepartment, error) {
	var departments []model.AigcDepartment
	query := r.db.Model(&model.AigcDepartment{})
	if value := strings.TrimSpace(keyword); value != "" {
		pattern := "%" + strings.ToLower(value) + "%"
		query = query.Where("lower(name) LIKE ?", pattern)
	}
	return departments, query.Order("created_at asc, name asc").Find(&departments).Error
}

func (r *Repository) AigcDepartment(id int64) (*model.AigcDepartment, error) {
	var department model.AigcDepartment
	if err := r.db.First(&department, "dept_id = ?", id).Error; err != nil {
		return nil, err
	}
	return &department, nil
}

func (r *Repository) AigcDepartmentUserCount(id int64) (int64, error) {
	var count int64
	return count, r.db.Model(&model.User{}).Where("dept_id = ?", id).Count(&count).Error
}

func (r *Repository) AigcDepartmentNames(ids []int64) (map[int64]string, error) {
	result := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var departments []model.AigcDepartment
	if err := r.db.Select("dept_id", "name").Where("dept_id IN ?", ids).Find(&departments).Error; err != nil {
		return nil, err
	}
	for _, department := range departments {
		result[department.DeptID] = department.Name
	}
	return result, nil
}

func (r *Repository) AigcProjects(keyword string, status string, level string, parentID *int64, visibleDeptID *int64, limit int, offset int) ([]model.AigcProject, int64, error) {
	var projects []model.AigcProject
	var total int64
	query := r.db.Model(&model.AigcProject{})
	if value := strings.TrimSpace(keyword); value != "" {
		pattern := "%" + strings.ToLower(value) + "%"
		query = query.Where("lower(project_name) LIKE ?", pattern)
	}
	if status == "启用" || status == "禁用" {
		query = query.Where("status = ?", status)
	}
	if level == "1" || level == "2" {
		parsedLevel, _ := strconv.Atoi(level)
		query = query.Where("level = ?", parsedLevel)
	}
	if parentID != nil {
		query = query.Where("parent_id = ?", *parentID)
	}
	if visibleDeptID != nil {
		query = query.Where("level = ? OR dept_id = ?", 1, *visibleDeptID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("project_id asc").Limit(limit).Offset(offset).Find(&projects).Error; err != nil {
		return nil, 0, err
	}
	return projects, total, nil
}

func (r *Repository) AigcSecondLevelProjectsByDept(deptID int64) ([]model.AigcProject, error) {
	var projects []model.AigcProject
	err := r.db.Where("level = ? AND dept_id = ? AND status = ?", 2, deptID, "启用").Order("project_id asc").Find(&projects).Error
	return projects, err
}

func (r *Repository) AigcSecondLevelProjects() ([]model.AigcProject, error) {
	var projects []model.AigcProject
	err := r.db.Where("level = ?", 2).Order("project_id asc").Find(&projects).Error
	return projects, err
}

func (r *Repository) AigcProject(id int64) (*model.AigcProject, error) {
	var project model.AigcProject
	if err := r.db.First(&project, "project_id = ?", id).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *Repository) AigcProjectByName(name string) (*model.AigcProject, error) {
	var project model.AigcProject
	if err := r.db.First(&project, "project_name = ?", strings.TrimSpace(name)).Error; err != nil {
		return nil, err
	}
	return &project, nil
}
