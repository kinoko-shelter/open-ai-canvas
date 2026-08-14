package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

const defaultKOLLoginURL = "http://localhost:3333/sso_login"
const defaultKOLCallbackURL = "http://localhost:3000/auth/kol/callback"
const kolUserInfoURL = "http://localhost:3333/dev-api/sso/get_user_info"
const kolProvider = "kol"

var kolUsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

type KOLLoginRequest struct {
	KOLToken string `json:"kolToken"`
}

type kolUserInfoResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		User        map[string]any `json:"user"`
		Roles       []any          `json:"roles"`
		Permissions []any          `json:"permissions"`
	} `json:"data"`
}

type kolUserProfile struct {
	User        map[string]any
	Roles       []any
	Permissions []any
}

func (s *Service) BeginKOLLogin(nextPath string) (string, error) {
	target, err := url.Parse(defaultKOLLoginURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return "", errors.New("KOL 登录地址配置无效")
	}
	callbackURL, err := kolCallbackURL()
	if err != nil {
		return "", err
	}
	query := target.Query()
	query.Set("redirect_uri", callbackURL)
	query.Set("next", safeOAuthNext(nextPath))
	query.Set("app_name", "故事创作")
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func kolCallbackURL() (string, error) {
	value := strings.TrimSpace(os.Getenv("CANVAS_KOL_CALLBACK_URL"))
	if value == "" {
		value = defaultKOLCallbackURL
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", BadAuthRequest("KOL 回调地址配置无效")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackOAuthHost(parsed.Hostname())) {
		return "", BadAuthRequest("KOL 回调地址必须使用 HTTPS，本地回环地址可使用 HTTP")
	}
	return value, nil
}

func (s *Service) LoginFromKOL(req KOLLoginRequest) (*AuthSessionResult, error) {
	token := strings.TrimSpace(req.KOLToken)
	if token == "" {
		return nil, BadAuthRequest("kolToken不能为空")
	}
	profile, err := fetchKOLUser(token)
	if err != nil {
		return nil, err
	}
	subject := firstKOLField(profile.User, "userId", "user_id", "sysUserId", "sys_user_id", "id")
	if subject == "" {
		return nil, errors.New("KOL 用户信息缺少稳定用户 ID")
	}
	username := firstKOLField(profile.User, "username", "userName", "user_name", "loginName", "login_name")
	displayName := firstNonEmpty(firstKOLField(profile.User, "displayName", "nickName", "nickname", "name"), username, "KOL 用户")
	email := normalizeEmail(firstKOLField(profile.User, "email", "mail"))
	avatarURL := firstKOLField(profile.User, "avatar", "avatarUrl", "avatar_url")

	identity, err := s.repo.UserIdentity(kolProvider, subject)
	var user *model.User
	if err == nil {
		user, err = s.repo.User(identity.UserID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user, err = s.createKOLUserForIdentity(identity, username, displayName, email, avatarURL)
			if err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		identity.ProviderUsername = firstNonEmpty(username, identity.ProviderUsername)
		identity.AvatarURL = firstNonEmpty(avatarURL, identity.AvatarURL)
		identity.UpdatedAt = time.Now()
		if err := s.repo.Save(identity); err != nil {
			return nil, err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		user, identity, err = s.createKOLUser(subject, username, displayName, email, avatarURL)
		if err != nil {
			return nil, err
		}
		if err := s.repo.CreateOAuthUser(user, identity); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if user.Status != model.UserStatusActive {
		return nil, Forbidden("该账号已被禁用")
	}
	s.applyKOLUserFields(user, subject)
	if err := s.ensureSignupBonus(user.ID); err != nil {
		return nil, err
	}
	now := time.Now()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.repo.Save(user); err != nil {
		return nil, err
	}
	s.recordActivity(user.ID, "login", 1)
	return s.createAuthSession(user)
}

func fetchKOLUser(token string) (*kolUserProfile, error) {
	if _, err := ValidateOutboundURL(kolUserInfoURL); err != nil {
		return nil, err
	}
	body, err := json.Marshal(KOLLoginRequest{KOLToken: token})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, kolUserInfoURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	ApplyDefaultOutboundHeaders(req)
	resp, err := OutboundHTTPClient(20 * time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("KOL 用户信息请求失败：%w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("KOL 用户信息请求失败")
	}
	var payload kolUserInfoResponse
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return nil, errors.New("KOL 用户信息响应无效")
	}
	if payload.Code != 200 {
		if strings.TrimSpace(payload.Msg) != "" {
			return nil, Unauthorized(payload.Msg)
		}
		return nil, Unauthorized("kolToken无效或已过期")
	}
	if len(payload.Data.User) == 0 {
		return nil, errors.New("KOL 用户信息响应缺少用户数据")
	}
	return &kolUserProfile{User: payload.Data.User, Roles: payload.Data.Roles, Permissions: payload.Data.Permissions}, nil
}

func (s *Service) createKOLUser(subject string, providerUsername string, displayName string, email string, avatarURL string) (*model.User, *model.UserIdentity, error) {
	base := providerUsername
	if !kolUsernamePattern.MatchString(base) {
		base = "kol_" + shortSubject(subject)
	}
	username := base
	if existing, err := s.repo.UserByUsername(username); err == nil && existing != nil {
		username = "kol_" + shortSubject(subject)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if existing, err := s.repo.UserByUsername(username); err == nil && existing != nil {
		username = "kol_" + shortSubject(subject) + "_" + shortSubject(username)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if email != "" {
		if validateEmail(email) != nil {
			email = ""
		} else if existing, err := s.repo.UserByEmail(email); err == nil && existing != nil {
			// 第三方邮箱不用于自动合并本地账号，避免账号接管。
			email = ""
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
	}
	now := time.Now()
	user := &model.User{
		ID: newID(), Username: username, Email: email,
		DisplayName: normalizeDisplayName(displayName, username),
		Role:        model.UserRoleUser, Status: model.UserStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	s.applyKOLUserFields(user, subject)
	identity := &model.UserIdentity{
		ID: newID(), UserID: user.ID, Provider: kolProvider,
		Subject: subject, ProviderUsername: providerUsername, AvatarURL: avatarURL,
	}
	return user, identity, nil
}

func (s *Service) createKOLUserForIdentity(identity *model.UserIdentity, providerUsername string, displayName string, email string, avatarURL string) (*model.User, error) {
	user, _, err := s.createKOLUser(identity.Subject, providerUsername, displayName, email, avatarURL)
	if err != nil {
		return nil, err
	}
	identity.UserID = user.ID
	identity.ProviderUsername = providerUsername
	identity.AvatarURL = avatarURL
	identity.UpdatedAt = time.Now()
	// 手动删除本地用户可能留下第三方身份悬空，这里原子重建用户并回写绑定。
	if err := s.repo.RepairOAuthUserIdentity(user, identity); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) applyKOLUserFields(user *model.User, kolUserID string) {
	user.KOLUserID = strings.TrimSpace(kolUserID)
}

func firstKOLField(profile map[string]any, fields ...string) string {
	for _, field := range fields {
		if value, ok := profile[field]; ok && value != nil {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				return text
			}
		}
	}
	return ""
}
