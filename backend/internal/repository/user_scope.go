package repository

import (
	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

// UserDataScope 表示当前登录用户能访问的用户数据范围；团队管理角色只扩展到同 dept_id 用户。
type UserDataScope struct {
	UserID      string
	DeptID      *int64
	IncludeTeam bool
}

func PersonalUserDataScope(userID string) UserDataScope {
	return UserDataScope{UserID: userID}
}

func TeamUserDataScope(userID string, deptID *int64) UserDataScope {
	return UserDataScope{UserID: userID, DeptID: deptID, IncludeTeam: deptID != nil}
}

func (scope UserDataScope) apply(query *gorm.DB, table string) *gorm.DB {
	if scope.IncludeTeam && scope.DeptID != nil {
		alias := table + "_owner_scope"
		return query.Joins("JOIN users "+alias+" ON "+alias+".id = "+table+".user_id").Where(alias+".dept_id = ?", *scope.DeptID)
	}
	return query.Where(table+".user_id = ?", scope.UserID)
}

func UserDataScopeFor(user model.User) UserDataScope {
	if TeamDataRole(user.Role) {
		return TeamUserDataScope(user.ID, user.DeptID)
	}
	return PersonalUserDataScope(user.ID)
}

func TeamDataRole(role model.UserRole) bool {
	return role == model.UserRoleTeamLead || role == model.UserRoleOperationsManager
}
