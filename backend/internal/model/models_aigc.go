package model

import "time"

// AigcDepartment 是运营组织结构，与创作项目和外部 KOL 表隔离。
type AigcDepartment struct {
	DeptID    int64     `json:"deptId" gorm:"column:dept_id;primaryKey;autoIncrement"`
	ParentID  int64     `json:"parentId" gorm:"column:parent_id;not null;default:0;index;uniqueIndex:idx_aigc_dept_parent_name,priority:1"`
	Name      string    `json:"name" gorm:"size:120;uniqueIndex:idx_aigc_dept_parent_name,priority:2"`
	Status    string    `json:"status" gorm:"size:12;not null;default:启用;index"`
	Remark    string    `json:"remark" gorm:"size:500"`
	CreatedBy string    `json:"createdBy" gorm:"column:created_by;index;size:36"`
	UpdatedBy string    `json:"updatedBy" gorm:"column:updated_by;index;size:36"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"column:updated_at"`
}

func (AigcDepartment) TableName() string { return "aigc_dept" }

// AigcProject 是运营配置项目，不关联创作域 Project。
type AigcProject struct {
	ProjectID         int64     `json:"projectId" gorm:"column:project_id;primaryKey;autoIncrement"`
	ParentID          int64     `json:"parentId" gorm:"column:parent_id;not null;default:0;index"`
	DeptID            int64     `json:"deptId" gorm:"column:dept_id;not null;default:0;index"`
	FirstCategoryID   *int64    `json:"firstCategoryId,omitempty" gorm:"column:first_category_id;index"`
	FirstCategoryName string    `json:"firstCategoryName" gorm:"column:first_category_name;size:160;index"`
	ProjectName       string    `json:"projectName" gorm:"column:project_name;size:160;uniqueIndex:idx_aigc_project_project_name_unique"`
	ProjectNameOuter  string    `json:"projectNameOuter" gorm:"column:project_name_outer;size:160"`
	ProjectType       string    `json:"projectType" gorm:"column:project_type;size:120;not null;default:默认"`
	ProjectDesc       string    `json:"projectDesc" gorm:"column:project_desc;type:text"`
	Status            string    `json:"status" gorm:"size:12;index"`
	Level             int       `json:"level" gorm:"not null;default:1;index"`
	Remark            string    `json:"remark" gorm:"size:500"`
	CreatedBy         string    `json:"createdBy" gorm:"column:created_by;index;size:36"`
	UpdatedBy         string    `json:"updatedBy" gorm:"column:updated_by;index;size:36"`
	CreatedAt         time.Time `json:"createdAt" gorm:"column:created_at;index"`
	UpdatedAt         time.Time `json:"updatedAt" gorm:"column:updated_at"`
}

func (AigcProject) TableName() string { return "aigc_project" }
