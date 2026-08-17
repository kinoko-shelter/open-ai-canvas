package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const SessionCookieName = "open_ai_canvas_session"

const sessionMaxAge = 30 * 24 * time.Hour

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

type AuthError struct {
	Status  int
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

type RegisterRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	EmailCode   string `json:"emailCode"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type PublicAuthSettings struct {
	FirstUser           bool `json:"firstUser"`
	RegistrationEnabled bool `json:"registrationEnabled"`
	LinuxDOEnabled      bool `json:"linuxdoEnabled"`
	EmailEnabled        bool `json:"emailEnabled"`
	EmailCodeRequired   bool `json:"emailCodeRequired"`
}

type AuthSessionResult struct {
	User       AuthUser `json:"user"`
	Session    string   `json:"session"`
	MaxAgeSecs int      `json:"maxAgeSecs"`
}

// 身份切换时 User 是实际访问数据的用户，Impersonator 保留发起切换的管理员用于退出和审计。
type AuthSessionContext struct {
	User         *model.User
	Session      *model.AuthSession
	Impersonator *model.User
}

type AuthUser struct {
	model.User
	AvatarURL        string `json:"avatarUrl,omitempty"`
	DeptName         string `json:"deptName,omitempty"`
	IdentityProvider string `json:"identityProvider,omitempty"`
	IdentityID       string `json:"identityId,omitempty"`
	IdentityUsername string `json:"identityUsername,omitempty"`
}

func BadAuthRequest(message string) *AuthError {
	return &AuthError{Status: 400, Message: message}
}

func Unauthorized(message string) *AuthError {
	return &AuthError{Status: 401, Message: message}
}

func Forbidden(message string) *AuthError {
	return &AuthError{Status: 403, Message: message}
}

func (s *Service) PublicAuthSettings() (*PublicAuthSettings, error) {
	count, err := s.repo.UserCount()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return &PublicAuthSettings{FirstUser: true, RegistrationEnabled: true, LinuxDOEnabled: false}, nil
	}
	registrationEnabled, err := s.RegistrationEnabled()
	if err != nil {
		return nil, err
	}
	emailEnabled, err := s.EmailEnabled()
	if err != nil {
		return nil, err
	}
	return &PublicAuthSettings{FirstUser: false, RegistrationEnabled: registrationEnabled, LinuxDOEnabled: s.LinuxDOEnabled(), EmailEnabled: emailEnabled, EmailCodeRequired: true}, nil
}

func (s *Service) Register(req RegisterRequest) (*AuthSessionResult, error) {
	username := normalizeUsername(req.Username)
	email := normalizeEmail(req.Email)
	displayName := normalizeDisplayName(req.DisplayName, username)
	if err := validateUsername(username); err != nil {
		return nil, err
	}
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}
	if email != "" {
		if err := validateEmail(email); err != nil {
			return nil, err
		}
	}
	s.registrationMu.Lock()
	defer s.registrationMu.Unlock()
	count, err := s.repo.UserCount()
	if err != nil {
		return nil, err
	}
	var verifiedCode *model.EmailVerificationCode
	if count > 0 {
		registrationEnabled, err := s.RegistrationEnabled()
		if err != nil {
			return nil, err
		}
		if !registrationEnabled {
			return nil, Forbidden("管理员未开放新用户注册")
		}
		if email == "" {
			return nil, BadAuthRequest("请输入邮箱")
		}
		verifiedCode, err = s.VerifyRegistrationEmailCode(email, req.EmailCode)
		if err != nil {
			return nil, err
		}
	}
	if _, err := s.repo.UserByUsername(username); err == nil {
		return nil, BadAuthRequest("用户名已存在")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if email != "" {
		if _, err := s.repo.UserByEmail(email); err == nil {
			return nil, BadAuthRequest("邮箱已被注册")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	user := model.User{
		ID:           newID(),
		Username:     username,
		Email:        email,
		DisplayName:  displayName,
		Role:         model.UserRoleUser,
		Status:       model.UserStatusActive,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if count == 0 {
		user.Role = model.UserRoleAdmin
	}
	if verifiedCode != nil {
		if err := s.repo.CreateUserWithEmailVerification(&user, verifiedCode.ID, time.Now()); err != nil {
			return nil, err
		}
	} else if err := s.repo.Create(&user); err != nil {
		return nil, err
	}
	if err := s.ensureSignupBonus(user.ID); err != nil {
		return nil, err
	}
	return s.createAuthSession(&user)
}

func (s *Service) Login(req LoginRequest) (*AuthSessionResult, error) {
	account := strings.TrimSpace(req.Username)
	user, err := s.repo.UserByAccount(account)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, Unauthorized("用户名、邮箱或密码不正确")
		}
		return nil, err
	}
	if user.Status != model.UserStatusActive {
		return nil, Forbidden("该账号已被禁用")
	}
	if !verifyPassword(req.Password, user.PasswordHash) {
		return nil, Unauthorized("用户名、邮箱或密码不正确")
	}
	now := time.Now()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.repo.Save(user); err != nil {
		return nil, err
	}
	if err := s.ensureSignupBonus(user.ID); err != nil {
		return nil, err
	}
	s.recordActivity(user.ID, "login", 1)
	return s.createAuthSession(user)
}

func (s *Service) Logout(cookieValue string) error {
	sessionID, _ := parseSessionCookie(cookieValue)
	if sessionID == "" {
		return nil
	}
	return s.repo.DeleteAuthSession(sessionID)
}

func (s *Service) ChangePassword(user *model.User, req ChangePasswordRequest) (*AuthSessionResult, error) {
	if user == nil {
		return nil, Unauthorized("请先登录")
	}
	if !verifyPassword(req.CurrentPassword, user.PasswordHash) {
		return nil, Unauthorized("当前密码不正确")
	}
	if err := validatePassword(req.NewPassword); err != nil {
		return nil, err
	}
	passwordHash, err := hashPassword(req.NewPassword)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = passwordHash
	user.UpdatedAt = time.Now()
	if err := s.repo.UpdateUserPasswordAndDeleteSessions(user, ""); err != nil {
		return nil, err
	}
	return s.createAuthSession(user)
}

func (s *Service) CurrentUser(cookieValue string) (*model.User, error) {
	context, err := s.CurrentAuthSession(cookieValue)
	if err != nil {
		return nil, err
	}
	return context.User, nil
}

func (s *Service) CurrentAuthSession(cookieValue string) (*AuthSessionContext, error) {
	sessionID, token := parseSessionCookie(cookieValue)
	if sessionID == "" || token == "" {
		return nil, Unauthorized("请先登录")
	}
	session, err := s.repo.AuthSession(sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, Unauthorized("登录状态已失效")
		}
		return nil, err
	}
	if time.Now().After(session.ExpiresAt) || session.TokenHash != hashToken(token) {
		_ = s.repo.DeleteAuthSession(sessionID)
		return nil, Unauthorized("登录状态已失效")
	}
	user, err := s.repo.User(session.UserID)
	if err != nil {
		return nil, err
	}
	if user.Status != model.UserStatusActive {
		return nil, Forbidden("该账号已被禁用")
	}
	context := &AuthSessionContext{User: user, Session: session}
	if session.ImpersonatorUserID == "" {
		return context, nil
	}
	impersonator, err := s.repo.User(session.ImpersonatorUserID)
	if err != nil {
		_ = s.repo.DeleteAuthSession(sessionID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, Unauthorized("管理员身份已失效，请重新登录")
		}
		return nil, err
	}
	if impersonator.Status != model.UserStatusActive || impersonator.Role != model.UserRoleAdmin {
		_ = s.repo.DeleteAuthSession(sessionID)
		return nil, Unauthorized("管理员身份已失效，请重新登录")
	}
	context.Impersonator = impersonator
	return context, nil
}

func (s *Service) StartUserImpersonation(cookieValue string, targetID string) (*AuthSessionResult, error) {
	context, err := s.CurrentAuthSession(cookieValue)
	if err != nil {
		return nil, err
	}
	actor := context.User
	if err := s.RequirePrimaryAdmin(actor); err != nil {
		return nil, err
	}
	if context.Session.ImpersonatorUserID != "" {
		return nil, Forbidden("当前已处于身份切换状态")
	}
	target, err := s.repo.User(strings.TrimSpace(targetID))
	if err != nil {
		return nil, err
	}
	if target.ID == actor.ID {
		return nil, BadAuthRequest("不能进入当前管理员账号")
	}
	if target.Role != model.UserRoleUser {
		return nil, Forbidden("不能进入管理员账号")
	}
	if target.Status != model.UserStatusActive {
		return nil, Forbidden("只能进入已启用的普通用户账号")
	}
	result, nextSession, err := s.newAuthSession(target, actor.ID)
	if err != nil {
		return nil, err
	}
	event := &model.AdminAuditEvent{
		ID: newID(), ActorUserID: actor.ID, Action: "user.impersonation.start", TargetType: "user", TargetID: target.ID,
		Summary: "以管理员身份进入用户账号", MetadataJSON: `{"mode":"impersonation"}`, CreatedAt: time.Now(),
	}
	if err := s.repo.ReplaceAuthSessionWithAudit(context.Session.ID, nextSession, event); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) ExitUserImpersonation(cookieValue string) (*AuthSessionResult, error) {
	context, err := s.CurrentAuthSession(cookieValue)
	if err != nil {
		return nil, err
	}
	if context.Session.ImpersonatorUserID == "" || context.Impersonator == nil {
		return nil, BadAuthRequest("当前不在身份切换状态")
	}
	result, nextSession, err := s.newAuthSession(context.Impersonator, "")
	if err != nil {
		return nil, err
	}
	event := &model.AdminAuditEvent{
		ID: newID(), ActorUserID: context.Impersonator.ID, Action: "user.impersonation.exit", TargetType: "user", TargetID: context.User.ID,
		Summary: "退出用户身份并返回管理员账号", MetadataJSON: `{"mode":"impersonation"}`, CreatedAt: time.Now(),
	}
	if err := s.repo.ReplaceAuthSessionWithAudit(context.Session.ID, nextSession, event); err != nil {
		return nil, err
	}
	return result, nil
}

// 认证响应只补充当前用户自己的第三方公开身份，不把身份表或密钥字段暴露给其他列表接口。
func (s *Service) PublicAuthUser(user *model.User) (AuthUser, error) {
	result := AuthUser{User: *user}
	if user.DeptID != nil {
		department, err := s.repo.AigcDepartment(*user.DeptID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return AuthUser{}, err
		}
		if department != nil {
			result.DeptName = department.Name
		}
	}
	identity, err := s.repo.UserIdentityForUser(user.ID, "linuxdo")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err != nil {
		return AuthUser{}, err
	}
	result.AvatarURL = identity.AvatarURL
	result.IdentityProvider = identity.Provider
	result.IdentityID = identity.Subject
	result.IdentityUsername = identity.ProviderUsername
	return result, nil
}

func (s *Service) createAuthSession(user *model.User) (*AuthSessionResult, error) {
	result, session, err := s.newAuthSession(user, "")
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(session); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) newAuthSession(user *model.User, impersonatorUserID string) (*AuthSessionResult, *model.AuthSession, error) {
	publicUser, err := s.PublicAuthUser(user)
	if err != nil {
		return nil, nil, err
	}
	token := randomToken()
	now := time.Now()
	session := model.AuthSession{
		ID:                 newID(),
		UserID:             user.ID,
		ImpersonatorUserID: strings.TrimSpace(impersonatorUserID),
		TokenHash:          hashToken(token),
		ExpiresAt:          now.Add(sessionMaxAge),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return &AuthSessionResult{User: publicUser, Session: session.ID + "." + token, MaxAgeSecs: int(sessionMaxAge.Seconds())}, &session, nil
}

func hashPassword(password string) (string, error) {
	// bcrypt 的标准存储字符串包含 cost、随机 salt 和 hash；不需要额外数据库 salt 字段。
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func verifyPassword(password string, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return newID() + newID()
	}
	return hex.EncodeToString(b[:])
}

func parseSessionCookie(value string) (string, string) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func normalizeUsername(value string) string {
	return strings.TrimSpace(value)
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeDisplayName(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	runes := []rune(value)
	if len(runes) > 40 {
		value = string(runes[:40])
	}
	return value
}

func validateUsername(value string) error {
	if !usernamePattern.MatchString(value) {
		return BadAuthRequest("用户名需为 3-32 位字母、数字、下划线或连字符")
	}
	return nil
}

func validatePassword(value string) error {
	if len([]rune(value)) < 8 {
		return BadAuthRequest("密码至少 8 位")
	}
	return nil
}

func validateEmail(value string) error {
	if _, err := mail.ParseAddress(value); err != nil {
		return BadAuthRequest("邮箱格式不正确")
	}
	return nil
}
