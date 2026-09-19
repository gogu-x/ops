package instance

import (
	"errors"
	"testing"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
)

func TestServiceInstanceValidation(t *testing.T) {
	if err := Validate(model.ServiceInstance{Name: "game-1"}); err == nil {
		t.Fatal("expected service_type_id validation error")
	}
	if err := Validate(model.ServiceInstance{ServiceTypeID: "type-1", Name: "Game-1"}); err == nil {
		t.Fatal("expected lowercase name validation error")
	}
	if err := Validate(model.ServiceInstance{ServiceTypeID: "type-1", Name: "game-1"}); err == nil {
		t.Fatal("expected image required validation error")
	}
	if err := Validate(model.ServiceInstance{ServiceTypeID: "type-1", Name: "game-1", Image: "game:v1", Params: []model.ServiceParam{{Flag: "server-id", Value: "1"}}}); err == nil {
		t.Fatal("expected flag validation error")
	}
}

func seedInstance(t *testing.T, a *Actor) model.ServiceInstance {
	t.Helper()
	item := NewServiceInstance("type-1", "game-1", "game:v1", "", "", "", "", nil)
	if err := a.repo.Create(t.Context(), item); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestUpdateImageValidation(t *testing.T) {
	a := NewActor(conf.Config{})
	created := seedInstance(t, a)
	if _, err := a.updateImage(created.ID, ""); err == nil {
		t.Fatal("expected error for empty image")
	}
	if _, err := a.updateImage(created.ID, "game:v2"); err == nil {
		t.Fatal("expected error resolving host without configured repositories")
	}
	if _, err := a.updateImage("missing-id", "game:v2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestContainerActionRestartRedeploys(t *testing.T) {
	a := NewActor(conf.Config{})
	created := seedInstance(t, a)
	if err := a.containerAction(created.ID, "restart"); err == nil {
		t.Fatal("expected error resolving host without configured repositories")
	}
	if err := a.containerAction(created.ID, "start"); err == nil {
		t.Fatal("expected error without a running host actor")
	}
	if err := a.containerAction("missing-id", "restart"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
