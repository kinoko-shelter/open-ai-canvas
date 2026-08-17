package service

import (
	"context"
	"errors"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"gorm.io/gorm"
)

type KOLBoundUserInfo struct {
	LocalUserID string                  `json:"localUserId"`
	KOLUserID   string                  `json:"kolUserId"`
	KOLUser     *repository.KOLUserInfo `json:"kolUser,omitempty"`
}

func (s *Service) KOLUserInfo(ctx context.Context, user *model.User) (*KOLBoundUserInfo, error) {
	if user == nil {
		return nil, Unauthorized("请先登录")
	}
	if user.KOLUserID == "" {
		return nil, BadAuthRequest("当前用户未绑定 KOL 用户 ID")
	}
	if s.kolRepo == nil {
		return nil, &AuthError{Status: 503, Message: "KOL 数据库未配置"}
	}
	info, err := s.kolRepo.UserInfo(ctx, user.KOLUserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, BadAuthRequest("KOL 用户不存在或已删除")
	}
	if err != nil {
		return nil, err
	}
	return &KOLBoundUserInfo{LocalUserID: user.ID, KOLUserID: user.KOLUserID, KOLUser: info}, nil
}
