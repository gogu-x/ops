package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gogu-x/ops/ops/host"
	"github.com/gogu-x/ops/ops/instance"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/ops/ops/internal/project"
	"github.com/gogu-x/ops/ops/internal/service"
)

type requestFunc func(actor string, message interface{}) (interface{}, error)

func (f requestFunc) Request(actor string, message interface{}) (interface{}, error) {
	return f(actor, message)
}

func newTestService(gateway Gateway) (*Service, project.Repository, service.Repository, instance.Repository) {
	projects := project.NewMemoryRepository()
	serviceTypes := service.NewMemoryRepository()
	instances := instance.NewMemoryRepository()
	return New(gateway, projects, serviceTypes, instances), projects, serviceTypes, instances
}

func TestServiceTypeCRUDUsesRepositoriesAndValidatesHost(t *testing.T) {
	ctx := context.Background()
	gateway := requestFunc(func(actor string, message interface{}) (interface{}, error) {
		if actor != "ops-host" {
			return nil, errors.New("unexpected actor: " + actor)
		}
		return host.ListResponse{Hosts: []model.Host{{ID: "host-1"}}}, nil
	})
	app, projects, _, _ := newTestService(gateway)
	projectItem := project.NewProject("project", "", "")
	if err := projects.Create(ctx, projectItem); err != nil {
		t.Fatal(err)
	}

	created, err := app.CreateServiceType(ctx, model.ServiceType{ProjectID: projectItem.ID, HostID: "host-1", Name: "game"})
	if err != nil {
		t.Fatalf("CreateServiceType: %v", err)
	}
	items, err := app.ListServiceTypes(ctx)
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("ListServiceTypes = %#v, %v", items, err)
	}
}

func TestCreateServiceTypeStopsWhenProjectDoesNotExist(t *testing.T) {
	app, _, _, _ := newTestService(requestFunc(func(string, interface{}) (interface{}, error) {
		t.Fatal("host actor must not be called when project is missing")
		return nil, nil
	}))
	_, err := app.CreateServiceType(context.Background(), model.ServiceType{ProjectID: "missing", HostID: "host-1", Name: "game"})
	if err == nil || !strings.Contains(err.Error(), "project not found") {
		t.Fatalf("expected project-not-found error, got %v", err)
	}
}

func TestInstanceCRUDUsesRepository(t *testing.T) {
	ctx := context.Background()
	app, _, serviceTypes, _ := newTestService(requestFunc(func(string, interface{}) (interface{}, error) {
		return instance.ListResponse{}, nil
	}))
	typeItem := service.NewServiceType("project-1", "host-1", "game")
	if err := serviceTypes.Create(ctx, typeItem); err != nil {
		t.Fatal(err)
	}
	created, err := app.CreateInstance(ctx, model.ServiceInstance{ServiceTypeID: typeItem.ID, Name: "game-1", Image: "game:v1"})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created instance has no ID")
	}
}
