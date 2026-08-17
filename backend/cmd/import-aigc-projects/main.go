package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

const importer = "aigc-project-import"

type sourceProject struct {
	TeamDeptID         int64  `gorm:"column:team_dept_id"`
	TeamDeptName       string `gorm:"column:team_dept_name"`
	FirstCategoryCode  int64  `gorm:"column:first_category_code"`
	FirstCategoryName  string `gorm:"column:first_category_name"`
	FirstCategoryValue string `gorm:"column:first_category_value"`
	ProjectName        string `gorm:"column:project_name"`
	Status             int    `gorm:"column:status"`
}

type firstCategory struct {
	Name string
}

func main() {
	sourceDSN := strings.TrimSpace(os.Getenv("CANVAS_AIGC_MYSQL_DSN"))
	if sourceDSN == "" {
		log.Fatal("请配置 CANVAS_AIGC_MYSQL_DSN")
	}
	dataDir := env("CANVAS_BACKEND_DATA_DIR", "data")
	target, err := database.Open(database.Config{
		Driver:  env("CANVAS_DATABASE_DRIVER", "sqlite"),
		DSN:     os.Getenv("DATABASE_URL"),
		DataDir: dataDir,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := database.ConfigurePool(target); err != nil {
		log.Fatal(err)
	}
	if err := database.MigrateSchema(target); err != nil {
		log.Fatal(err)
	}
	source, err := database.Open(database.Config{Driver: "mysql", DSN: sourceDSN})
	if err != nil {
		log.Fatal(err)
	}
	if err := database.ConfigurePool(source); err != nil {
		log.Fatal(err)
	}

	var records []sourceProject
	if err := source.Raw(`
		SELECT team_dept_id, team_dept_name, first_category_code, first_category_name,
			first_category_value, project_name, status
		FROM aigc_secondary_project
		WHERE is_deleted = 0
		ORDER BY first_category_code, project_name, team_dept_id
	`).Scan(&records).Error; err != nil {
		log.Fatal(err)
	}
	if len(records) == 0 {
		log.Fatal("来源没有可导入的项目记录")
	}

	var createdDepartments, createdFirstProjects, createdSecondProjects int
	if err := target.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		departments := make(map[int64]string)
		categories := make(map[int64]firstCategory)
		for _, record := range records {
			departments[record.TeamDeptID] = strings.TrimSpace(record.TeamDeptName)
			categories[record.FirstCategoryCode] = firstCategory{
				Name: strings.TrimSpace(record.FirstCategoryName),
			}
		}
		departmentIDs := make(map[int64]int64, len(departments))
		for _, deptID := range sortedKeys(departments) {
			name := departments[deptID]
			if name == "" {
				return fmt.Errorf("来源团队 %d 缺少名称", deptID)
			}
			var department model.AigcDepartment
			err := tx.First(&department, "parent_id = ? AND name = ?", 0, name).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				department = model.AigcDepartment{ParentID: 0, Name: name, Status: "启用", CreatedBy: importer, UpdatedBy: importer, CreatedAt: now, UpdatedAt: now}
				if err := tx.Create(&department).Error; err != nil {
					return err
				}
				createdDepartments++
			} else if err != nil {
				return err
			} else if err := tx.Model(&model.AigcDepartment{}).Where("dept_id = ?", department.DeptID).Updates(map[string]any{
				"name": name, "updated_by": importer, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			departmentIDs[deptID] = department.DeptID
		}

		parentIDs := make(map[int64]int64, len(categories))
		categoryCodes := make([]int64, 0, len(categories))
		for code := range categories {
			categoryCodes = append(categoryCodes, code)
		}
		sort.Slice(categoryCodes, func(i, j int) bool { return categoryCodes[i] < categoryCodes[j] })
		for _, code := range categoryCodes {
			category := categories[code]
			if category.Name == "" {
				return fmt.Errorf("一级分类 %d 缺少名称", code)
			}
			rootDeptID := int64(0)
			var project model.AigcProject
			err := tx.First(&project, "project_name = ?", category.Name).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				project = model.AigcProject{
					DeptID: rootDeptID, ProjectName: category.Name, FirstCategoryName: category.Name,
					ProjectType: "默认", Status: "启用", Level: 1,
					CreatedBy: importer, UpdatedBy: importer, CreatedAt: now, UpdatedAt: now,
				}
				if err := tx.Create(&project).Error; err != nil {
					return err
				}
				project.FirstCategoryID = &project.ProjectID
				if err := tx.Model(&model.AigcProject{}).Where("project_id = ?", project.ProjectID).Updates(map[string]any{
					"first_category_id": project.ProjectID,
				}).Error; err != nil {
					return err
				}
				createdFirstProjects++
			} else if err != nil {
				return err
			} else {
				if err := tx.Model(&model.AigcProject{}).Where("project_id = ?", project.ProjectID).Updates(map[string]any{
					"parent_id": 0, "dept_id": rootDeptID, "first_category_id": project.ProjectID,
					"first_category_name": category.Name, "project_name_outer": "",
					"project_type": "默认", "status": "启用", "level": 1,
					"updated_by": importer, "updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
			parentIDs[code] = project.ProjectID
		}

		for _, record := range records {
			parentID := parentIDs[record.FirstCategoryCode]
			deptID := departmentIDs[record.TeamDeptID]
			projectName := strings.TrimSpace(record.ProjectName)
			if parentID == 0 || deptID == 0 || projectName == "" {
				return fmt.Errorf("来源二级项目数据不完整：一级分类=%d，项目=%q", record.FirstCategoryCode, record.ProjectName)
			}
			status := "禁用"
			if record.Status == 1 {
				status = "启用"
			}
			var project model.AigcProject
			err := tx.First(&project, "project_name = ?", projectName).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				project = model.AigcProject{
					ParentID: parentID, DeptID: deptID, FirstCategoryID: &parentID, FirstCategoryName: categories[record.FirstCategoryCode].Name,
					ProjectName: projectName, ProjectType: "默认",
					Status: status, Level: 2, CreatedBy: importer, UpdatedBy: importer, CreatedAt: now, UpdatedAt: now,
				}
				if err := tx.Create(&project).Error; err != nil {
					return err
				}
				createdSecondProjects++
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&model.AigcProject{}).Where("project_id = ?", project.ProjectID).Updates(map[string]any{
				"parent_id": parentID, "dept_id": deptID, "first_category_id": parentID,
				"first_category_name": categories[record.FirstCategoryCode].Name,
				"project_name_outer":  "", "project_type": "默认", "status": status, "level": 2,
				"updated_by": importer, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}
	log.Printf("导入完成：来源=%d，新增团队=%d，新增一级项目=%d，新增二级项目=%d", len(records), createdDepartments, createdFirstProjects, createdSecondProjects)
}

func env(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func sortedKeys(values map[int64]string) []int64 {
	keys := make([]int64, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
