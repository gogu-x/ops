package project

import (
	"context"
	"errors"
	"testing"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
)

func TestProjectLifecycle(t *testing.T) {
	a := NewActor(conf.Config{})
	created, err := a.create(model.Project{
		Name:    "game-project",
		Note:    "游戏业务线",
		EnvVars: "--etcd etcd:2379 \\\n--mongo-url mongodb://172.17.0.1:27018",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.EnvVars == "" {
		t.Fatalf("unexpected project: %+v", created)
	}
	if _, err := a.create(model.Project{Name: "game-project"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	created.Note = "更新后的备注"
	updated, err := a.update(created)
	if err != nil || updated.Note != "更新后的备注" {
		t.Fatalf("update failed: %+v %v", updated, err)
	}
	if err := a.repo.Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestProjectValidation(t *testing.T) {
	if err := validate(model.Project{Name: ""}); err == nil {
		t.Fatal("expected name required validation error")
	}
}
