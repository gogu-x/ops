package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gogu-x/ops/ops/host"
	"github.com/gogu-x/ops/ops/instance"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/ops/ops/internal/project"
	"github.com/gogu-x/ops/ops/internal/service"
)

// Gateway is the boundary between synchronous application use cases and the
// actor runtime. Keeping it explicit makes orchestration testable without a
// global actor registry.
type Gateway interface {
	Request(actor string, message interface{}) (interface{}, error)
}

type Service struct {
	gateway      Gateway
	projects     project.Repository
	serviceTypes service.Repository
	instances    instance.Repository
}

func New(gateway Gateway, projects project.Repository, serviceTypes service.Repository, instances instance.Repository) *Service {
	return &Service{gateway: gateway, projects: projects, serviceTypes: serviceTypes, instances: instances}
}

func (s *Service) ListHosts() ([]model.Host, error) {
	value, err := s.gateway.Request("ops-host", host.ListRequest{})
	if err != nil {
		return nil, err
	}
	result, ok := value.(host.ListResponse)
	if !ok {
		return nil, errors.New("invalid host actor response")
	}
	return result.Hosts, nil
}

func (s *Service) CreateHost(item model.Host) (model.Host, error) {
	value, err := s.gateway.Request("ops-host", host.CreateRequest{Host: item})
	if err != nil {
		return model.Host{}, err
	}
	result, ok := value.(model.Host)
	if !ok {
		return model.Host{}, errors.New("invalid host actor response")
	}
	return result, nil
}

func (s *Service) DeleteHost(id string) error {
	_, err := s.gateway.Request("ops-host", host.DeleteRequest{ID: id})
	return err
}

func (s *Service) TestHost(id string) (host.TestResponse, error) {
	value, err := s.gateway.Request("ops-host", host.TestRequest{ID: id})
	if err != nil {
		return host.TestResponse{}, err
	}
	result, ok := value.(host.TestResponse)
	if !ok {
		return host.TestResponse{}, errors.New("invalid host actor response")
	}
	return result, nil
}

func (s *Service) ListProjects(ctx context.Context) ([]model.Project, error) {
	return s.projects.List(ctx)
}

func (s *Service) CreateProject(ctx context.Context, item model.Project) (model.Project, error) {
	if err := project.Validate(item); err != nil {
		return model.Project{}, err
	}
	created := project.NewProject(item.Name, item.Note, item.EnvVars)
	if err := s.projects.Create(ctx, created); err != nil {
		return model.Project{}, err
	}
	return created, nil
}

func (s *Service) UpdateProject(ctx context.Context, item model.Project) (model.Project, error) {
	if err := project.Validate(item); err != nil {
		return model.Project{}, err
	}
	existing, err := s.projects.Get(ctx, item.ID)
	if err != nil {
		return model.Project{}, err
	}
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	if err := s.projects.Update(ctx, item); err != nil {
		return model.Project{}, err
	}
	return item, nil
}

func (s *Service) DeleteProject(ctx context.Context, id string) error {
	return s.projects.Delete(ctx, id)
}

func (s *Service) ListServiceTypes(ctx context.Context) ([]model.ServiceType, error) {
	return s.serviceTypes.List(ctx)
}

func (s *Service) CreateServiceType(ctx context.Context, item model.ServiceType) (model.ServiceType, error) {
	if err := s.validateServiceTypeReferences(ctx, item); err != nil {
		return model.ServiceType{}, err
	}
	if err := service.Validate(item); err != nil {
		return model.ServiceType{}, err
	}
	created := service.NewServiceType(item.ProjectID, item.HostID, item.Name)
	if err := s.serviceTypes.Create(ctx, created); err != nil {
		return model.ServiceType{}, err
	}
	return created, nil
}

func (s *Service) UpdateServiceType(ctx context.Context, item model.ServiceType) (model.ServiceType, error) {
	if err := s.validateServiceTypeReferences(ctx, item); err != nil {
		return model.ServiceType{}, err
	}
	if err := service.Validate(item); err != nil {
		return model.ServiceType{}, err
	}
	existing, err := s.serviceTypes.Get(ctx, item.ID)
	if err != nil {
		return model.ServiceType{}, err
	}
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	if err := s.serviceTypes.Update(ctx, item); err != nil {
		return model.ServiceType{}, err
	}
	return item, nil
}

func (s *Service) DeleteServiceType(ctx context.Context, id string) error {
	return s.serviceTypes.Delete(ctx, id)
}

func (s *Service) validateServiceTypeReferences(ctx context.Context, item model.ServiceType) error {
	if strings.TrimSpace(item.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return err
	}
	if !containsProject(projects, item.ProjectID) {
		return errors.New("project not found: " + item.ProjectID)
	}
	if strings.TrimSpace(item.HostID) == "" {
		return errors.New("host_id is required")
	}
	hosts, err := s.ListHosts()
	if err != nil {
		return err
	}
	if !containsHost(hosts, item.HostID) {
		return errors.New("host not found: " + item.HostID)
	}
	return nil
}

func (s *Service) ListInstances(ctx context.Context) ([]model.ServiceInstance, error) {
	items, err := s.instances.List(ctx)
	if err != nil {
		return nil, err
	}
	value, err := s.gateway.Request("ops-instance", instance.EnrichRequest{ServiceInstances: items})
	if err != nil {
		return items, nil
	}
	result, ok := value.(instance.ListResponse)
	if !ok {
		return nil, errors.New("invalid service instance actor response")
	}
	return result.ServiceInstances, nil
}

func (s *Service) CreateInstance(ctx context.Context, item model.ServiceInstance) (model.ServiceInstance, error) {
	if err := s.ensureServiceType(ctx, item.ServiceTypeID); err != nil {
		return model.ServiceInstance{}, err
	}
	if err := instance.Validate(item); err != nil {
		return model.ServiceInstance{}, err
	}
	created := instance.NewServiceInstance(item.ServiceTypeID, item.Name, item.Image, item.Note, item.EnvText, item.Network, item.PortMapping, item.Params)
	if err := s.instances.Create(ctx, created); err != nil {
		return model.ServiceInstance{}, err
	}
	return created, nil
}

func (s *Service) UpdateInstance(ctx context.Context, item model.ServiceInstance) (model.ServiceInstance, error) {
	if err := s.ensureServiceType(ctx, item.ServiceTypeID); err != nil {
		return model.ServiceInstance{}, err
	}
	if err := instance.Validate(item); err != nil {
		return model.ServiceInstance{}, err
	}
	existing, err := s.instances.Get(ctx, item.ID)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	if err := s.instances.Update(ctx, item); err != nil {
		return model.ServiceInstance{}, err
	}
	return item, nil
}

func (s *Service) DeleteInstance(ctx context.Context, id string) error {
	_ = s.RemoveContainer(id)
	return s.instances.Delete(ctx, id)
}

func (s *Service) ensureServiceType(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("service_type_id is required")
	}
	items, err := s.ListServiceTypes(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == id {
			return nil
		}
	}
	return errors.New("service type not found: " + id)
}

func (s *Service) instanceResult(message interface{}) (model.ServiceInstance, error) {
	value, err := s.gateway.Request("ops-instance", message)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	result, ok := value.(model.ServiceInstance)
	if !ok {
		return model.ServiceInstance{}, errors.New("invalid service instance actor response")
	}
	return result, nil
}

func (s *Service) DeployInstance(id string) (model.ServiceInstance, error) {
	return s.instanceResult(instance.DeployRequest{ID: id})
}

func (s *Service) UpdateInstanceImage(id, image string) (model.ServiceInstance, error) {
	return s.instanceResult(instance.UpdateImageRequest{ID: id, Image: image})
}

func (s *Service) InstanceStatus(id string) (model.ServiceInstance, error) {
	return s.instanceResult(instance.StatusRequest{ID: id})
}

func (s *Service) ContainerAction(id, action string) error {
	_, err := s.gateway.Request("ops-instance", instance.ContainerActionRequest{ID: id, Action: action})
	return err
}

func (s *Service) RemoveContainer(id string) error {
	_, err := s.gateway.Request("ops-instance", instance.RemoveContainerRequest{ID: id})
	return err
}

func (s *Service) InstanceDetail(id string) (instance.ContainerDetail, error) {
	value, err := s.gateway.Request("ops-instance", instance.DetailRequest{ID: id})
	if err != nil {
		return instance.ContainerDetail{}, err
	}
	result, ok := value.(instance.DetailResponse)
	if !ok {
		return instance.ContainerDetail{}, errors.New("invalid service instance actor response")
	}
	return result.Detail, nil
}

func (s *Service) InstanceLogs(id, tail string) (string, error) {
	value, err := s.gateway.Request("ops-instance", instance.LogsRequest{ID: id, Tail: tail})
	if err != nil {
		return "", err
	}
	result, ok := value.(instance.LogsResponse)
	if !ok {
		return "", errors.New("invalid service instance actor response")
	}
	return result.Logs, nil
}

func containsProject(items []model.Project, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func containsHost(items []model.Host, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
