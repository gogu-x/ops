package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gogu-x/ops/admin/internal/environment"
	"github.com/gogu-x/ops/admin/internal/project"
	"github.com/gogu-x/ops/admin/internal/service"
	"github.com/gogu-x/ops/instance"
	"github.com/gogu-x/ops/model"
)

type requestFunc func(actor string, message interface{}) (interface{}, error)

func (f requestFunc) Request(actor string, message interface{}) (interface{}, error) {
	return f(actor, message)
}

func newTestService(gateway Gateway) (*Service, project.Repository, environment.Repository, service.Repository, InstanceRepository) {
	projects := project.NewMemoryRepository()
	environments := environment.NewMemoryRepository()
	serviceTypes := service.NewMemoryRepository()
	instances := instance.NewMemoryRepository()
	return New(gateway, projects, environments, serviceTypes, instances), projects, environments, serviceTypes, instances
}

func TestServiceTypeCRUDUsesRepositoriesAndValidatesHost(t *testing.T) {
	ctx := context.Background()
	gateway := requestFunc(func(actor string, message interface{}) (interface{}, error) {
		if actor != "ops-host" {
			return nil, errors.New("unexpected actor: " + actor)
		}
		return model.HostListResponse{Hosts: []model.Host{{ID: "host-1"}}}, nil
	})
	app, projects, environments, _, _ := newTestService(gateway)
	projectItem := project.NewProject("project", "", "")
	if err := projects.Create(ctx, projectItem); err != nil {
		t.Fatal(err)
	}
	envItem := environment.NewEnvironment(projectItem.ID, "beta")
	if err := environments.Create(ctx, envItem); err != nil {
		t.Fatal(err)
	}

	created, err := app.CreateServiceType(ctx, model.ServiceType{ProjectID: projectItem.ID, EnvironmentID: envItem.ID, Name: "game"})
	if err != nil {
		t.Fatalf("CreateServiceType: %v", err)
	}
	items, err := app.ListServiceTypes(ctx)
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("ListServiceTypes = %#v, %v", items, err)
	}
}

func TestCreateServiceTypeStopsWhenProjectDoesNotExist(t *testing.T) {
	app, _, _, _, _ := newTestService(requestFunc(func(string, interface{}) (interface{}, error) {
		t.Fatal("host actor must not be called when project is missing")
		return nil, nil
	}))
	_, err := app.CreateServiceType(context.Background(), model.ServiceType{ProjectID: "missing", EnvironmentID: "env-1", Name: "game"})
	if err == nil || !strings.Contains(err.Error(), "project not found") {
		t.Fatalf("expected project-not-found error, got %v", err)
	}
}

func TestCreateServiceTypeStopsWhenEnvironmentDoesNotExist(t *testing.T) {
	ctx := context.Background()
	app, projects, _, _, _ := newTestService(requestFunc(func(string, interface{}) (interface{}, error) {
		t.Fatal("host actor must not be called when environment is missing")
		return nil, nil
	}))
	projectItem := project.NewProject("project", "", "")
	if err := projects.Create(ctx, projectItem); err != nil {
		t.Fatal(err)
	}
	_, err := app.CreateServiceType(ctx, model.ServiceType{ProjectID: projectItem.ID, EnvironmentID: "missing", Name: "game"})
	if err == nil || !strings.Contains(err.Error(), "environment not found") {
		t.Fatalf("expected environment-not-found error, got %v", err)
	}
}

func TestInstanceCRUDUsesRepository(t *testing.T) {
	ctx := context.Background()
	app, projects, environments, serviceTypes, _ := newTestService(requestFunc(func(actor string, message interface{}) (interface{}, error) {
		if actor == "ops-host" {
			return model.HostListResponse{Hosts: []model.Host{
				{ID: "host-1", ProjectIDs: []string{"project-id", "another-project"}},
				{ID: "host-2", ProjectIDs: []string{"another-project"}},
			}}, nil
		}
		return model.ListResponse{}, nil
	}))
	projectItem := project.NewProject("project", "", "")
	projectItem.ID = "project-id"
	if err := projects.Create(ctx, projectItem); err != nil {
		t.Fatal(err)
	}
	envItem := environment.NewEnvironment(projectItem.ID, "beta")
	if err := environments.Create(ctx, envItem); err != nil {
		t.Fatal(err)
	}
	typeItem := service.NewServiceType(projectItem.ID, envItem.ID, "game")
	if err := serviceTypes.Create(ctx, typeItem); err != nil {
		t.Fatal(err)
	}
	created, err := app.CreateInstance(ctx, model.ServiceInstance{ServiceTypeID: typeItem.ID, HostID: "host-1", Name: "game-1", Image: "game:v1"})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created instance has no ID")
	}
	if _, err := app.CreateInstance(ctx, model.ServiceInstance{ServiceTypeID: typeItem.ID, HostID: "host-2", Name: "game-2", Image: "game:v1"}); !errors.Is(err, model.ErrHostProjectConflict) {
		t.Fatalf("expected host from another project to be rejected, got %v", err)
	}
}

func TestSetProjectHostsAllowsSharingHostUsedByAnotherProject(t *testing.T) {
	ctx := context.Background()
	var binding model.SetProjectHostsRequest
	app, projects, _, serviceTypes, instances := newTestService(requestFunc(func(actor string, message interface{}) (interface{}, error) {
		if actor != "ops-host" {
			return nil, errors.New("unexpected actor: " + actor)
		}
		switch request := message.(type) {
		case model.ListRequest:
			return model.HostListResponse{Hosts: []model.Host{{ID: "host-1", ProjectID: "project-1"}}}, nil
		case model.SetProjectHostsRequest:
			binding = request
			return true, nil
		default:
			return nil, errors.New("unexpected host request")
		}
	}))
	for _, id := range []string{"project-1", "project-2"} {
		item := project.NewProject(id, "", "")
		item.ID = id
		if err := projects.Create(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	typeItem := service.NewServiceType("project-1", "environment-1", "game")
	if err := serviceTypes.Create(ctx, typeItem); err != nil {
		t.Fatal(err)
	}
	usedInstance := model.NewServiceInstance(typeItem.ID, "host-1", "game-1", "game:v1", "", "", "", "", nil)
	if err := instances.Create(ctx, usedInstance); err != nil {
		t.Fatal(err)
	}

	if err := app.SetProjectHosts(ctx, "project-2", []string{"host-1"}); err != nil {
		t.Fatalf("binding a host already used by another project's service instance: %v", err)
	}
	if binding.ProjectID != "project-2" || len(binding.HostIDs) != 1 || binding.HostIDs[0] != "host-1" {
		t.Fatalf("unexpected host binding request: %+v", binding)
	}
}
