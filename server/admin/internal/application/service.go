package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogu-x/ops/admin/internal/environment"
	"github.com/gogu-x/ops/admin/internal/project"
	"github.com/gogu-x/ops/admin/internal/service"
	"github.com/gogu-x/ops/model"
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
	environments environment.Repository
	serviceTypes service.Repository
	instances    InstanceRepository
}

// InstanceRepository is the subset of the service-instance repository
// needed by the application layer. Declared locally (rather than importing
// instance/internal) because Go's internal-package visibility rule forbids
// importing another module-relative internal package from a sibling package
// tree.
type InstanceRepository interface {
	List(ctx context.Context) ([]model.ServiceInstance, error)
	Get(ctx context.Context, id string) (model.ServiceInstance, error)
	Create(ctx context.Context, item model.ServiceInstance) error
	Update(ctx context.Context, item model.ServiceInstance) error
	Delete(ctx context.Context, id string) error
}

func New(gateway Gateway, projects project.Repository, environments environment.Repository, serviceTypes service.Repository, instances InstanceRepository) *Service {
	return &Service{gateway: gateway, projects: projects, environments: environments, serviceTypes: serviceTypes, instances: instances}
}

func (s *Service) ListHosts() ([]model.Host, error) {
	value, err := s.gateway.Request("ops-host", model.ListRequest{})
	if err != nil {
		return nil, err
	}
	result, ok := value.(model.HostListResponse)
	if !ok {
		return nil, errors.New("invalid host actor response")
	}
	return result.Hosts, nil
}

func (s *Service) CreateHost(item model.Host) (model.Host, error) {
	value, err := s.gateway.Request("ops-host", model.CreateRequest{Host: item})
	if err != nil {
		return model.Host{}, err
	}
	result, ok := value.(model.Host)
	if !ok {
		return model.Host{}, errors.New("invalid host actor response")
	}
	return result, nil
}

func (s *Service) DeleteHost(ctx context.Context, id string) error {
	hosts, err := s.ListHosts()
	if err != nil {
		return err
	}
	for _, host := range hosts {
		if host.ID == id {
			if host.HasProjectBindings() {
				return model.ErrHostBound
			}
			instances, err := s.instances.List(ctx)
			if err != nil {
				return err
			}
			for _, instance := range instances {
				if instance.HostID == id {
					return model.ErrHostInUse
				}
			}
			_, err = s.gateway.Request("ops-host", model.DeleteRequest{ID: id})
			return err
		}
	}
	return model.ErrNotFound
}

func (s *Service) SetProjectHosts(ctx context.Context, projectID string, hostIDs []string) error {
	if strings.TrimSpace(projectID) == "" {
		return errors.New("project_id is required")
	}
	projectID = strings.TrimSpace(projectID)
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return err
	}

	desired := make(map[string]struct{}, len(hostIDs))
	normalizedHostIDs := make([]string, 0, len(hostIDs))
	for _, id := range hostIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New("host_id is required")
		}
		if _, exists := desired[id]; !exists {
			desired[id] = struct{}{}
			normalizedHostIDs = append(normalizedHostIDs, id)
		}
	}

	hosts, err := s.ListHosts()
	if err != nil {
		return err
	}
	existingHosts := make(map[string]model.Host, len(hosts))
	removed := make(map[string]struct{})
	for _, host := range hosts {
		existingHosts[host.ID] = host
		if host.HasProject(projectID) {
			if _, keep := desired[host.ID]; !keep {
				removed[host.ID] = struct{}{}
			}
		}
	}
	for id := range desired {
		_, ok := existingHosts[id]
		if !ok {
			return model.ErrNotFound
		}
	}

	if len(removed) > 0 {
		instances, err := s.instances.List(ctx)
		if err != nil {
			return err
		}
		serviceTypes, err := s.serviceTypes.List(ctx)
		if err != nil {
			return err
		}
		projectByType := make(map[string]string, len(serviceTypes))
		for _, item := range serviceTypes {
			projectByType[item.ID] = item.ProjectID
		}
		for _, instance := range instances {
			if _, isRemoved := removed[instance.HostID]; isRemoved && projectByType[instance.ServiceTypeID] == projectID {
				return model.ErrHostInUse
			}
		}
	}

	_, err = s.gateway.Request("ops-host", model.SetProjectHostsRequest{ProjectID: projectID, HostIDs: normalizedHostIDs})
	return err
}

func (s *Service) TestHost(id string) (model.TestResponse, error) {
	value, err := s.gateway.Request("ops-host", model.TestRequest{ID: id})
	if err != nil {
		return model.TestResponse{}, err
	}
	result, ok := value.(model.TestResponse)
	if !ok {
		return model.TestResponse{}, errors.New("invalid host actor response")
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
	hosts, err := s.ListHosts()
	if err != nil {
		return err
	}
	for _, host := range hosts {
		if host.HasProject(id) {
			return model.ErrProjectHasBoundHosts
		}
	}
	return s.projects.Delete(ctx, id)
}

func (s *Service) ListEnvironments(ctx context.Context) ([]model.Environment, error) {
	return s.environments.List(ctx)
}

func (s *Service) CreateEnvironment(ctx context.Context, item model.Environment) (model.Environment, error) {
	if err := s.validateEnvironmentReferences(ctx, item); err != nil {
		return model.Environment{}, err
	}
	if err := environment.Validate(item); err != nil {
		return model.Environment{}, err
	}
	created := environment.NewEnvironment(item.ProjectID, item.Name)
	if err := s.environments.Create(ctx, created); err != nil {
		return model.Environment{}, err
	}
	return created, nil
}

func (s *Service) UpdateEnvironment(ctx context.Context, item model.Environment) (model.Environment, error) {
	if err := s.validateEnvironmentReferences(ctx, item); err != nil {
		return model.Environment{}, err
	}
	if err := environment.Validate(item); err != nil {
		return model.Environment{}, err
	}
	existing, err := s.environments.Get(ctx, item.ID)
	if err != nil {
		return model.Environment{}, err
	}
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	if err := s.environments.Update(ctx, item); err != nil {
		return model.Environment{}, err
	}
	return item, nil
}

func (s *Service) DeleteEnvironment(ctx context.Context, id string) error {
	return s.environments.Delete(ctx, id)
}

func (s *Service) validateEnvironmentReferences(ctx context.Context, item model.Environment) error {
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
	return nil
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
	created := service.NewServiceType(item.ProjectID, item.EnvironmentID, item.Name)
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
	if existing.ProjectID != item.ProjectID {
		instances, err := s.instances.List(ctx)
		if err != nil {
			return model.ServiceType{}, err
		}
		for _, instance := range instances {
			if instance.ServiceTypeID == item.ID {
				return model.ServiceType{}, errors.New("cannot move a service type to another project while it has instances")
			}
		}
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
	if strings.TrimSpace(item.EnvironmentID) == "" {
		return errors.New("environment_id is required")
	}
	env, err := s.environments.Get(ctx, item.EnvironmentID)
	if err != nil {
		return errors.New("environment not found: " + item.EnvironmentID)
	}
	if env.ProjectID != item.ProjectID {
		return errors.New("environment does not belong to project: " + item.EnvironmentID)
	}
	return nil
}

func (s *Service) ListInstances(ctx context.Context) ([]model.ServiceInstance, error) {
	items, err := s.instances.List(ctx)
	if err != nil {
		return nil, err
	}
	value, err := s.gateway.Request("ops-instance", model.EnrichRequest{ServiceInstances: items})
	if err != nil {
		return items, nil
	}
	result, ok := value.(model.ListResponse)
	if !ok {
		return nil, errors.New("invalid service instance actor response")
	}
	return result.ServiceInstances, nil
}

func (s *Service) CreateInstance(ctx context.Context, item model.ServiceInstance) (model.ServiceInstance, error) {
	if err := model.ValidateServiceInstance(item); err != nil {
		return model.ServiceInstance{}, err
	}
	serviceType, err := s.getServiceType(ctx, item.ServiceTypeID)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	if err := s.ensureHostInProject(item.HostID, serviceType.ProjectID); err != nil {
		return model.ServiceInstance{}, err
	}
	created := model.NewServiceInstance(item.ServiceTypeID, item.HostID, item.Name, item.Image, item.Note, item.EnvText, item.Network, item.PortMapping, item.Params)
	created.RestartPolicy = model.NormalizeRestartPolicy(item.RestartPolicy)
	if err := s.instances.Create(ctx, created); err != nil {
		return model.ServiceInstance{}, err
	}
	return created, nil
}

func (s *Service) UpdateInstance(ctx context.Context, item model.ServiceInstance) (model.ServiceInstance, error) {
	if err := model.ValidateServiceInstance(item); err != nil {
		return model.ServiceInstance{}, err
	}
	serviceType, err := s.getServiceType(ctx, item.ServiceTypeID)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	if err := s.ensureHostInProject(item.HostID, serviceType.ProjectID); err != nil {
		return model.ServiceInstance{}, err
	}
	existing, err := s.instances.Get(ctx, item.ID)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	item.RestartPolicy = model.NormalizeRestartPolicy(item.RestartPolicy)
	if err := s.instances.Update(ctx, item); err != nil {
		return model.ServiceInstance{}, err
	}
	return item, nil
}

func (s *Service) DeleteInstance(ctx context.Context, id string) error {
	_ = s.RemoveContainer(id)
	return s.instances.Delete(ctx, id)
}

func (s *Service) getServiceType(ctx context.Context, id string) (model.ServiceType, error) {
	if strings.TrimSpace(id) == "" {
		return model.ServiceType{}, errors.New("service_type_id is required")
	}
	items, err := s.ListServiceTypes(ctx)
	if err != nil {
		return model.ServiceType{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return model.ServiceType{}, errors.New("service type not found: " + id)
}

func (s *Service) ensureHostInProject(hostID, projectID string) error {
	hosts, err := s.ListHosts()
	if err != nil {
		return err
	}
	for _, host := range hosts {
		if host.ID != hostID {
			continue
		}
		if !host.HasProjectBindings() {
			return fmt.Errorf("%w: host is not bound to a project", model.ErrHostUnresolved)
		}
		if !host.HasProject(projectID) {
			return fmt.Errorf("%w: host does not belong to the service type project", model.ErrHostProjectConflict)
		}
		return nil
	}
	return model.ErrHostUnresolved
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

func (s *Service) DeployInstance(ctx context.Context, id string) (model.ServiceInstance, error) {
	if err := s.ensureStoredInstanceHost(ctx, id); err != nil {
		return model.ServiceInstance{}, err
	}
	return s.instanceResult(model.DeployRequest{ID: id})
}

func (s *Service) UpdateInstanceImage(ctx context.Context, id, image string) (model.ServiceInstance, error) {
	if err := s.ensureStoredInstanceHost(ctx, id); err != nil {
		return model.ServiceInstance{}, err
	}
	return s.instanceResult(model.UpdateImageRequest{ID: id, Image: image})
}

func (s *Service) ensureStoredInstanceHost(ctx context.Context, id string) error {
	item, err := s.instances.Get(ctx, id)
	if err != nil {
		return err
	}
	serviceType, err := s.getServiceType(ctx, item.ServiceTypeID)
	if err != nil {
		return err
	}
	return s.ensureHostInProject(item.HostID, serviceType.ProjectID)
}

func (s *Service) InstanceStatus(id string) (model.ServiceInstance, error) {
	return s.instanceResult(model.StatusRequest{ID: id})
}

func (s *Service) ContainerAction(id, action string) error {
	_, err := s.gateway.Request("ops-instance", model.InstanceContainerActionRequest{ID: id, Action: action})
	return err
}

func (s *Service) RemoveContainer(id string) error {
	_, err := s.gateway.Request("ops-instance", model.RemoveContainerRequest{ID: id})
	return err
}

func (s *Service) InstanceDetail(id string) (model.ContainerDetail, error) {
	value, err := s.gateway.Request("ops-instance", model.DetailRequest{ID: id})
	if err != nil {
		return model.ContainerDetail{}, err
	}
	result, ok := value.(model.DetailResponse)
	if !ok {
		return model.ContainerDetail{}, errors.New("invalid service instance actor response")
	}
	return result.Detail, nil
}

func (s *Service) InstanceLogs(id, tail string) (string, error) {
	value, err := s.gateway.Request("ops-instance", model.LogsRequest{ID: id, Tail: tail})
	if err != nil {
		return "", err
	}
	result, ok := value.(model.LogsResponse)
	if !ok {
		return "", errors.New("invalid service instance actor response")
	}
	return result.Logs, nil
}

func (s *Service) InstanceEvents(id string, limit int) ([]model.InstanceEvent, error) {
	value, err := s.gateway.Request("ops-instance", model.ListEventsRequest{ID: id, Limit: limit})
	if err != nil {
		return nil, err
	}
	result, ok := value.(model.EventsResponse)
	if !ok {
		return nil, errors.New("invalid service instance actor response")
	}
	return result.Events, nil
}

func containsProject(items []model.Project, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
