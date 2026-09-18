package instance

import (
	"context"
	"errors"
	"testing"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
)

func TestServiceInstanceLifecycle(t *testing.T) {
	a := NewActor(conf.Config{})
	created, err := a.create(model.ServiceInstance{
		ServiceTypeID: "type-1",
		Name:          "game-1",
		Image:         "gogs-game:v1",
		Params:        []model.ServiceParam{{Flag: "--server-id", Value: "1"}},
		EnvText:       "--etcd etcd:2379 \\\n--mongo-url mongodb://172.17.0.1:27018",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || len(created.Params) != 1 || created.EnvText == "" || created.Image == "" {
		t.Fatalf("unexpected service instance: %+v", created)
	}
	if _, err := a.create(model.ServiceInstance{ServiceTypeID: "type-1", Name: "game-1", Image: "gogs-game:v1"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	// same name allowed under a different service type
	if _, err := a.create(model.ServiceInstance{ServiceTypeID: "type-2", Name: "game-1", Image: "gogs-game:v1"}); err != nil {
		t.Fatalf("expected success for different service type scope, got %v", err)
	}
	created.Note = "更新备注"
	updated, err := a.update(created)
	if err != nil || updated.Note != "更新备注" {
		t.Fatalf("update failed: %+v %v", updated, err)
	}
	if err := a.repo.Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceInstanceValidation(t *testing.T) {
	if err := validate(model.ServiceInstance{Name: "game-1"}); err == nil {
		t.Fatal("expected service_type_id validation error")
	}
	if err := validate(model.ServiceInstance{ServiceTypeID: "type-1", Name: "Game-1"}); err == nil {
		t.Fatal("expected lowercase name validation error")
	}
	if err := validate(model.ServiceInstance{ServiceTypeID: "type-1", Name: "game-1"}); err == nil {
		t.Fatal("expected image required validation error")
	}
	if err := validate(model.ServiceInstance{ServiceTypeID: "type-1", Name: "game-1", Image: "gogs-game:v1", Params: []model.ServiceParam{{Flag: "server-id", Value: "1"}}}); err == nil {
		t.Fatal("expected flag validation error")
	}
}

func TestUpdateImageValidation(t *testing.T) {
	a := NewActor(conf.Config{})
	created, err := a.create(model.ServiceInstance{
		ServiceTypeID: "type-1",
		Name:          "game-1",
		Image:         "gogs-game:v1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := a.updateImage(created.ID, ""); err == nil {
		t.Fatal("expected error for empty image")
	}
	if _, err := a.updateImage(created.ID, "   "); err == nil {
		t.Fatal("expected error for blank image")
	}
	// No host/service actors are registered in this unit test, so resolving
	// the instance's host fails; updateImage should surface that error
	// rather than silently succeeding.
	if _, err := a.updateImage(created.ID, "gogs-game:v2"); err == nil {
		t.Fatal("expected error resolving host without a running actor tree")
	}
	if _, err := a.updateImage("missing-id", "gogs-game:v2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing instance, got %v", err)
	}
}

func TestContainerActionRestartRedeploys(t *testing.T) {
	a := NewActor(conf.Config{})
	created, err := a.create(model.ServiceInstance{
		ServiceTypeID: "type-1",
		Name:          "game-1",
		Image:         "gogs-game:v1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// "restart" must go through deploy() (which resolves the host via the
	// service actor) rather than looking up an existing container directly;
	// with no actor tree registered in this unit test, both paths fail, but
	// the specific error differs and lets us tell them apart.
	err = a.containerAction(created.ID, "restart")
	if err == nil {
		t.Fatal("expected error resolving host without a running actor tree")
	}
	if errors.Is(err, ErrHostUnresolved) {
		t.Fatalf("resolveHostID should fail on missing service actor, not ErrHostUnresolved: %v", err)
	}

	// start/stop/remove still go through lookupContainerID (bare Docker
	// actions on the existing container), which fails differently (host
	// actor unavailable) since they never call deploy().
	if err := a.containerAction(created.ID, "start"); err == nil {
		t.Fatal("expected error for start without a running actor tree")
	}
	if err := a.containerAction("missing-id", "restart"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing instance, got %v", err)
	}
}
