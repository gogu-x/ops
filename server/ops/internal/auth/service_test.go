package auth

import (
	"context"
	"testing"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginRefreshAndLogout(t *testing.T) {
	cfg := conf.Config{JWTSecret: "test-secret", AccessTTLMin: 15, RefreshTTLDays: 7}
	repo := store.NewMemoryRepository()
	service := NewService(repo, cfg)
	if err := service.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}

	password := "test-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(context.Background(), store.NewUser("tester", string(hash), "admin")); err != nil {
		t.Fatal(err)
	}

	first, err := service.Login(context.Background(), "tester", password)
	if err != nil {
		t.Fatal(err)
	}
	if first.AccessToken == "" || first.RefreshToken == "" {
		t.Fatal("tokens should be issued")
	}
	if _, err := service.ParseAccessToken(first.AccessToken); err != nil {
		t.Fatal(err)
	}

	second, err := service.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token should rotate")
	}
	if _, err := service.Refresh(context.Background(), first.RefreshToken); err == nil {
		t.Fatal("old refresh token should be revoked")
	}
	if err := service.Logout(context.Background(), second.RefreshToken); err != nil {
		t.Fatal(err)
	}
}
