package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gogu-x/ops/admin/internal/store"
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/model"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID   string `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Type     string `json:"type"`
	jwt.RegisteredClaims
}

type Service struct {
	repo store.Repository
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,64}$`)

func (s *Service) ListUsers(ctx context.Context) ([]model.User, error) { return s.repo.ListUsers(ctx) }

func (s *Service) CreateManagedUser(ctx context.Context, user model.User, password string) (model.User, error) {
	user.Username = strings.TrimSpace(user.Username)
	if !usernamePattern.MatchString(user.Username) {
		return model.User{}, errors.New("用户名需为 3-64 位字母、数字、点、下划线或短横线")
	}
	if len(password) < 8 {
		return model.User{}, errors.New("密码至少需要 8 位")
	}
	if err := normalizeAccess(&user); err != nil {
		return model.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, err
	}
	created := store.NewUser(user.Username, string(hash), user.Role)
	created.ProjectIDs = user.ProjectIDs
	created.Permissions = user.Permissions
	created.Disabled = user.Disabled
	if err := s.repo.CreateUser(ctx, created); err != nil {
		return model.User{}, err
	}
	return created, nil
}

func (s *Service) UpdateManagedUser(ctx context.Context, user model.User) (model.User, error) {
	existing, err := s.repo.FindUserByID(ctx, user.ID)
	if err != nil {
		return model.User{}, err
	}
	user.Username = strings.TrimSpace(user.Username)
	if !usernamePattern.MatchString(user.Username) {
		return model.User{}, errors.New("用户名需为 3-64 位字母、数字、点、下划线或短横线")
	}
	if err := normalizeAccess(&user); err != nil {
		return model.User{}, err
	}
	if existing.Role == model.RoleAdmin && !existing.Disabled && (user.Role != model.RoleAdmin || user.Disabled) {
		users, err := s.repo.ListUsers(ctx)
		if err != nil {
			return model.User{}, err
		}
		admins := 0
		for _, item := range users {
			if item.Role == model.RoleAdmin && !item.Disabled {
				admins++
			}
		}
		if admins <= 1 {
			return model.User{}, errors.New("至少保留一个启用的管理员")
		}
	}
	user.PasswordHash = existing.PasswordHash
	user.CreatedAt = existing.CreatedAt
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return model.User{}, err
	}
	return user, nil
}

func (s *Service) ResetManagedUserPassword(ctx context.Context, id, password string) error {
	if len(password) < 8 {
		return errors.New("密码至少需要 8 位")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.SetUserPasswordHash(ctx, id, string(hash))
}

func normalizeAccess(user *model.User) error {
	if user.Role == "" {
		user.Role = model.RoleUser
	}
	if user.Role != model.RoleAdmin && user.Role != model.RoleUser {
		return fmt.Errorf("不支持的用户角色")
	}
	projects := make([]string, 0, len(user.ProjectIDs))
	seenProjects := make(map[string]bool)
	for _, id := range user.ProjectIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seenProjects[id] {
			seenProjects[id] = true
			projects = append(projects, id)
		}
	}
	user.ProjectIDs = projects
	permissions := make([]string, 0, len(user.Permissions))
	seenPermissions := make(map[string]bool)
	for _, permission := range user.Permissions {
		valid := false
		for _, known := range model.UserPermissions {
			if permission == known {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("不支持的权限: %s", permission)
		}
		if !seenPermissions[permission] {
			seenPermissions[permission] = true
			permissions = append(permissions, permission)
		}
	}
	user.Permissions = permissions
	if user.Role == model.RoleUser && len(user.ProjectIDs) == 0 {
		return errors.New("普通用户至少需要分配一个项目")
	}
	if user.Role == model.RoleUser && len(user.Permissions) == 0 {
		return errors.New("普通用户至少需要一项功能权限")
	}
	return nil
}

func NewService(repo store.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) FindUserByID(ctx context.Context, id string) (model.User, error) {
	return s.repo.FindUserByID(ctx, id)
}

func (s *Service) Bootstrap(ctx context.Context) error {
	if indexer, ok := s.repo.(interface{ EnsureIndexes(context.Context) error }); ok {
		if err := indexer.EnsureIndexes(ctx); err != nil {
			return err
		}
	}
	count, err := s.repo.CountUsers(ctx)
	if err != nil || count > 0 {
		return err
	}
	password := conf.AdminPassword
	if password == "" {
		password = conf.RandomSecret()[:16]
		println("[ops] OPS_ADMIN_PASSWORD 未设置，默认 admin 密码:", password)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.CreateUser(ctx, store.NewUser("admin", string(hash), "admin"))
}

func (s *Service) Login(ctx context.Context, username, password string) (model.TokenResponse, error) {
	user, err := s.repo.FindUser(ctx, username)
	if err != nil || user.Disabled || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return model.TokenResponse{}, errors.New("invalid credentials")
	}
	return s.issueTokens(ctx, user)
}

func (s *Service) Refresh(ctx context.Context, raw string) (model.TokenResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.TokenResponse{}, errors.New("refresh token is required")
	}
	token, err := s.repo.ConsumeRefreshToken(ctx, hashToken(raw), time.Now().UTC())
	if err != nil {
		return model.TokenResponse{}, errors.New("invalid refresh token")
	}
	user, err := s.repo.FindUserByID(ctx, token.UserID)
	if err != nil || user.Disabled {
		return model.TokenResponse{}, errors.New("user is unavailable")
	}
	return s.issueTokens(ctx, user)
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	_, err := s.repo.ConsumeRefreshToken(ctx, hashToken(raw), time.Now().UTC())
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}

func (s *Service) ParseAccessToken(raw string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(conf.JWTSecret), nil
	})
	if err != nil || !token.Valid || claims.Type != "access" {
		return nil, errors.New("invalid access token")
	}
	return claims, nil
}

func (s *Service) issueTokens(ctx context.Context, user model.User) (model.TokenResponse, error) {
	now := time.Now().UTC()
	accessExpiry := now.Add(time.Duration(conf.AccessTTLMin) * time.Minute)
	accessClaims := Claims{UserID: user.ID, Username: user.Username, Role: user.Role, Type: "access", RegisteredClaims: jwt.RegisteredClaims{Subject: user.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(accessExpiry)}}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(conf.JWTSecret))
	if err != nil {
		return model.TokenResponse{}, err
	}
	rawRefresh := conf.RandomSecret() + conf.RandomSecret()
	refreshExpiry := now.Add(time.Duration(conf.RefreshTTLDays) * 24 * time.Hour)
	if err := s.repo.SaveRefreshToken(ctx, model.RefreshToken{ID: conf.RandomSecret()[:16], UserID: user.ID, TokenHash: hashToken(rawRefresh), ExpiresAt: refreshExpiry, CreatedAt: now}); err != nil {
		return model.TokenResponse{}, err
	}
	return model.TokenResponse{AccessToken: access, RefreshToken: rawRefresh, ExpiresIn: int64(time.Until(accessExpiry).Seconds()), User: user}, nil
}

func hashToken(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
