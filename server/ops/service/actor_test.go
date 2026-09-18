package service

import (
	"context"
	"errors"
	"testing"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
)

func TestServiceTypeLifecycle(t *testing.T) {
	a := NewActor(conf.Config{})
	created, err := a.create(model.ServiceType{
		ProjectID: "project-1",
		HostID:    "host-1",
		Name:      "game",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatalf("unexpected service type: %+v", created)
	}
	if _, err := a.create(model.ServiceType{ProjectID: "project-1", HostID: "host-1", Name: "game"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	created.Name = "game"
	updated, err := a.update(created)
	if err != nil || updated.Name != "game" {
		t.Fatalf("update failed: %+v %v", updated, err)
	}
	if err := a.repo.Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceTypeValidation(t *testing.T) {
	if err := validate(model.ServiceType{HostID: "host-1", Name: "game"}); err == nil {
		t.Fatal("expected project_id validation error")
	}
	if err := validate(model.ServiceType{ProjectID: "project-1", HostID: "host-1", Name: "Game"}); err == nil {
		t.Fatal("expected lowercase name validation error")
	}
}
