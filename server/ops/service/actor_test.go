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
		HostID: "host-1",
		Name:   "game",
		Image:  "gogs-game:v1",
		Params: []model.ServiceParam{{Flag: "--server-id", Value: "{{id}}"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || len(created.Params) != 1 {
		t.Fatalf("unexpected service type: %+v", created)
	}
	if _, err := a.create(model.ServiceType{HostID: "host-1", Name: "game", Image: "gogs-game:v2"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	created.Image = "gogs-game:v2"
	updated, err := a.update(created)
	if err != nil || updated.Image != "gogs-game:v2" {
		t.Fatalf("update failed: %+v %v", updated, err)
	}
	if err := a.repo.Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceTypeValidation(t *testing.T) {
	if err := validate(model.ServiceType{HostID: "host-1", Name: "Game", Image: "image"}); err == nil {
		t.Fatal("expected lowercase name validation error")
	}
	if err := validate(model.ServiceType{HostID: "host-1", Name: "game", Image: "image", Params: []model.ServiceParam{{Flag: "server-id", Value: "1"}}}); err == nil {
		t.Fatal("expected flag validation error")
	}
}
