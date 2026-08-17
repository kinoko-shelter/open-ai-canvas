package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type KOLRepository struct {
	db *gorm.DB
}

type KOLUserInfo struct {
	UserID      string        `json:"userId"`
	UserName    string        `json:"userName"`
	NickName    string        `json:"nickName"`
	Email       string        `json:"email,omitempty"`
	PhoneNumber string        `json:"phoneNumber,omitempty"`
	Avatar      string        `json:"avatar,omitempty"`
	Status      string        `json:"status"`
	Dept        *KOLDeptInfo  `json:"dept,omitempty"`
	Roles       []KOLRoleInfo `json:"roles"`
}

type KOLDeptInfo struct {
	DeptID   string `json:"deptId"`
	DeptName string `json:"deptName"`
}

type KOLRoleInfo struct {
	RoleID   string `json:"roleId"`
	RoleName string `json:"roleName"`
	RoleKey  string `json:"roleKey"`
}

func NewKOLRepository(db *gorm.DB) *KOLRepository {
	if db == nil {
		return nil
	}
	return &KOLRepository{db: db}
}

func (r *KOLRepository) UserInfo(ctx context.Context, kolUserID string) (*KOLUserInfo, error) {
	kolUserID = strings.TrimSpace(kolUserID)
	if r == nil || r.db == nil {
		return nil, errors.New("KOL 数据库未配置")
	}
	if kolUserID == "" {
		return nil, errors.New("当前用户未绑定 KOL 用户 ID")
	}
	var row struct {
		UserID      string `gorm:"column:user_id"`
		UserName    string `gorm:"column:user_name"`
		NickName    string `gorm:"column:nick_name"`
		Email       string `gorm:"column:email"`
		PhoneNumber string `gorm:"column:phonenumber"`
		Avatar      string `gorm:"column:avatar"`
		Status      string `gorm:"column:status"`
		DeptID      string `gorm:"column:dept_id"`
		DeptName    string `gorm:"column:dept_name"`
	}
	err := r.db.WithContext(ctx).Raw(`
SELECT
  CAST(u.user_id AS CHAR) AS user_id,
  u.user_name,
  u.nick_name,
  COALESCE(u.email, '') AS email,
  COALESCE(u.phonenumber, '') AS phonenumber,
  COALESCE(u.avatar, '') AS avatar,
  COALESCE(u.status, '') AS status,
  COALESCE(CAST(u.dept_id AS CHAR), '') AS dept_id,
  COALESCE(d.dept_name, '') AS dept_name
FROM sys_user u
LEFT JOIN sys_dept d ON d.dept_id = u.dept_id AND COALESCE(d.del_flag, '0') = '0'
WHERE u.user_id = ? AND COALESCE(u.del_flag, '0') = '0'
LIMIT 1`, kolUserID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.UserID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var roleRows []struct {
		RoleID   string `gorm:"column:role_id"`
		RoleName string `gorm:"column:role_name"`
		RoleKey  string `gorm:"column:role_key"`
	}
	if err := r.db.WithContext(ctx).Raw(`
SELECT
  CAST(r.role_id AS CHAR) AS role_id,
  r.role_name,
  r.role_key
FROM sys_user_role ur
JOIN sys_role r ON r.role_id = ur.role_id
WHERE ur.user_id = ? AND COALESCE(r.del_flag, '0') = '0' AND COALESCE(r.status, '0') = '0'
ORDER BY r.role_sort ASC, r.role_id ASC`, kolUserID).Scan(&roleRows).Error; err != nil {
		return nil, err
	}
	result := &KOLUserInfo{
		UserID:      row.UserID,
		UserName:    row.UserName,
		NickName:    row.NickName,
		Email:       row.Email,
		PhoneNumber: row.PhoneNumber,
		Avatar:      row.Avatar,
		Status:      row.Status,
		Roles:       make([]KOLRoleInfo, 0, len(roleRows)),
	}
	if row.DeptID != "" {
		result.Dept = &KOLDeptInfo{DeptID: row.DeptID, DeptName: row.DeptName}
	}
	for _, role := range roleRows {
		result.Roles = append(result.Roles, KOLRoleInfo{RoleID: role.RoleID, RoleName: role.RoleName, RoleKey: role.RoleKey})
	}
	return result, nil
}
