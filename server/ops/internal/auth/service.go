package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/ops/ops/internal/store"
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
	cfg  conf.Config
}

func NewService(repo store.Repository, cfg conf.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

func (s *Service) Bootstrap(ctx context.Context) error {
	count, err := s.repo.CountUsers(ctx)
	if err != nil || count > 0 {
		return err
	}
	password := s.cfg.AdminPassword
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
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid || claims.Type != "access" {
		return nil, errors.New("invalid access token")
	}
	return claims, nil
}

func (s *Service) issueTokens(ctx context.Context, user model.User) (model.TokenResponse, error) {
	now := time.Now().UTC()
	accessExpiry := now.Add(time.Duration(s.cfg.AccessTTLMin) * time.Minute)
	accessClaims := Claims{UserID: user.ID, Username: user.Username, Role: user.Role, Type: "access", RegisteredClaims: jwt.RegisteredClaims{Subject: user.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(accessExpiry)}}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return model.TokenResponse{}, err
	}
	rawRefresh := conf.RandomSecret() + conf.RandomSecret()
	refreshExpiry := now.Add(time.Duration(s.cfg.RefreshTTLDays) * 24 * time.Hour)
	if err := s.repo.SaveRefreshToken(ctx, model.RefreshToken{ID: conf.RandomSecret()[:16], UserID: user.ID, TokenHash: hashToken(rawRefresh), ExpiresAt: refreshExpiry, CreatedAt: now}); err != nil {
		return model.TokenResponse{}, err
	}
	return model.TokenResponse{AccessToken: access, RefreshToken: rawRefresh, ExpiresIn: int64(time.Until(accessExpiry).Seconds()), User: user}, nil
}

func hashToken(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
