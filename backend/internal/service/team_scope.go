package service

import (
	"errors"
	"strings"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/gorm"
)

func (s *Service) dataScope(actor *model.User) (repository.UserDataScope, error) {
	if actor == nil {
		return repository.UserDataScope{}, Unauthorized("请先登录")
	}
	if actor.Role == model.UserRoleTeamLead {
		if actor.DeptID == nil {
			return repository.UserDataScope{}, Forbidden("团队管理账号未设置团队")
		}
		return repository.TeamUserDataScope(actor.ID, actor.DeptID), nil
	}
	if actor.Role == model.UserRoleOperationsManager && actor.DeptID != nil {
		return repository.TeamUserDataScope(actor.ID, actor.DeptID), nil
	}
	return repository.PersonalUserDataScope(actor.ID), nil
}

func (s *Service) actorForUserID(userID string) (*model.User, error) {
	return s.repo.User(strings.TrimSpace(userID))
}

func (s *Service) dataScopeForUserID(userID string) (repository.UserDataScope, error) {
	actor, err := s.actorForUserID(userID)
	if err != nil {
		return repository.UserDataScope{}, err
	}
	return s.dataScope(actor)
}

func (s *Service) canAccessOwnedUser(actor *model.User, ownerID string) error {
	ownerID = strings.TrimSpace(ownerID)
	if actor == nil {
		return Unauthorized("请先登录")
	}
	if ownerID == "" {
		return gorm.ErrRecordNotFound
	}
	if actor.ID == ownerID {
		return nil
	}
	if !repository.TeamDataRole(actor.Role) {
		return gorm.ErrRecordNotFound
	}
	if actor.DeptID == nil {
		if actor.Role == model.UserRoleTeamLead {
			return Forbidden("团队管理账号未设置团队")
		}
		return gorm.ErrRecordNotFound
	}
	owner, err := s.repo.User(ownerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return gorm.ErrRecordNotFound
	}
	if err != nil {
		return err
	}
	if owner.DeptID == nil || *owner.DeptID != *actor.DeptID {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) scopedTask(actor *model.User, id string) (*model.Task, error) {
	task, err := s.repo.Task(strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := s.canAccessOwnedUser(actor, task.UserID); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *Service) scopedAsset(actor *model.User, id string) (*model.Asset, error) {
	asset, err := s.repo.Asset(strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := s.canAccessOwnedUser(actor, asset.UserID); err != nil {
		return nil, err
	}
	return asset, nil
}

func (s *Service) scopedCanvasProject(actor *model.User, id string) (*model.CanvasProject, error) {
	project, err := s.repo.CanvasProject(strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := s.canAccessOwnedUser(actor, project.UserID); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *Service) scopedProject(actor *model.User, id string) (*model.Project, error) {
	project, err := s.repo.Project(strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := s.canAccessOwnedUser(actor, project.UserID); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *Service) projectForUserID(userID string, id string) (*model.Project, error) {
	actor, err := s.actorForUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.scopedProject(actor, id)
}

func (s *Service) assetForUserID(userID string, id string) (*model.Asset, error) {
	actor, err := s.actorForUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.scopedAsset(actor, id)
}

func (s *Service) canvasProjectForUserID(userID string, id string) (*model.CanvasProject, error) {
	actor, err := s.actorForUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.scopedCanvasProject(actor, id)
}

func (s *Service) taskForUserID(userID string, id string) (*model.Task, error) {
	actor, err := s.actorForUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.scopedTask(actor, id)
}
