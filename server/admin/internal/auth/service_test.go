package auth

import (
	"context"
	"testing"

	"github.com/gogu-x/ops/admin/internal/store"
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/model"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginRefreshAndLogout(t *testing.T) {
	conf.JWTSecret = "test-secret"
	conf.AccessTTLMin = 15
	conf.RefreshTTLDays = 7
	repo := store.NewMemoryRepository()
	service := NewService(repo)
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

func TestManagedUsersRequireScopeAndProtectLastAdmin(t *testing.T) {
	conf.JWTSecret = "test-secret"
	conf.AccessTTLMin = 15
	conf.RefreshTTLDays = 7
	ctx := context.Background()
	repo := store.NewMemoryRepository()
	service := NewService(repo)
	if err := service.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateManagedUser(ctx, model.User{Username: "viewer", Role: model.RoleUser}, "viewer-password"); err == nil {
		t.Fatal("creating a user without assigned projects and permissions should fail")
	}

	admin, err := repo.FindUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UpdateManagedUser(ctx, model.User{ID: admin.ID, Username: admin.Username, Role: model.RoleUser, ProjectIDs: []string{"project-a"}, Permissions: []string{model.PermissionProjectsView}})
	if err == nil {
		t.Fatal("the last enabled admin should not be demoted")
	}

	created, err := service.CreateManagedUser(ctx, model.User{Username: "viewer", Role: model.RoleUser, ProjectIDs: []string{"project-a"}, Permissions: []string{model.PermissionServicesManage, model.PermissionDashboardView}}, "viewer-password")
	if err != nil {
		t.Fatal(err)
	}
	if !created.HasProject("project-a") || !created.HasPermission(model.PermissionServicesView) || !created.HasPermission(model.PermissionDashboardView) {
		t.Fatalf("managed user access not normalized: %#v", created)
	}
	if _, err := service.CreateManagedUser(ctx, model.User{Username: "viewer", Role: model.RoleUser, ProjectIDs: []string{"project-a"}, Permissions: []string{model.PermissionProjectsView}}, "viewer-password"); err == nil {
		t.Fatal("duplicate usernames should be rejected")
	}
}
