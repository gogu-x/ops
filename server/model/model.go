package model

import (
	"strings"
	"time"
)

type User struct {
	ID           string    `json:"id" bson:"_id"`
	Username     string    `json:"username" bson:"username"`
	PasswordHash string    `json:"-" bson:"password_hash"`
	Role         string    `json:"role" bson:"role"`
	ProjectIDs   []string  `json:"project_ids" bson:"project_ids"`
	Permissions  []string  `json:"permissions" bson:"permissions"`
	Disabled     bool      `json:"disabled" bson:"disabled"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
}

const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	PermissionDashboardView             = "dashboard.view"
	PermissionProjectsView              = "projects.view"
	PermissionServicesView              = "services.view"
	PermissionServicesManage            = "services.manage"
	PermissionServiceTypesView          = "service_types.view"
	PermissionServiceTypesManage        = "service_types.manage"
	PermissionRolloutTemplatesManage    = "rollout_templates.manage"
	PermissionHostsView                 = "hosts.view"
	PermissionHostsManage               = "hosts.manage"
	PermissionServicesCreate            = "services.create"
	PermissionServicesEdit              = "services.edit"
	PermissionServicesDelete            = "services.delete"
	PermissionServicesDeploy            = "services.deploy"
	PermissionServicesStart             = "services.start"
	PermissionServicesStop              = "services.stop"
	PermissionServicesRestart           = "services.restart"
	PermissionServicesUpdateImage       = "services.update_image"
	PermissionServicesRemoveContainer   = "services.remove_container"
	PermissionServicesRollback          = "services.rollback"
	PermissionServicesBatch             = "services.batch"
	PermissionServicesEnvironmentCreate = "services.environment_create"
	PermissionServicesEnvironmentEdit   = "services.environment_edit"
	PermissionServicesEnvironmentDelete = "services.environment_delete"
	PermissionServicesRolloutCreate     = "services.rollout_create"
	PermissionServicesRolloutPause      = "services.rollout_pause"
	PermissionServicesRolloutResume     = "services.rollout_resume"
	PermissionServicesRolloutRetry      = "services.rollout_retry"
	PermissionServicesAutoCreate        = "services.auto_create"
	PermissionServicesAutoEdit          = "services.auto_edit"
	PermissionServicesAutoEnable        = "services.auto_enable"
	PermissionServicesAutoDisable       = "services.auto_disable"
	PermissionHostsCreate               = "hosts.create"
	PermissionHostsEdit                 = "hosts.edit"
	PermissionHostsTest                 = "hosts.test"
	PermissionHostsDelete               = "hosts.delete"
	PermissionServiceTypesCreate        = "service_types.create"
	PermissionServiceTypesEdit          = "service_types.edit"
	PermissionServiceTypesDelete        = "service_types.delete"
)

var UserPermissions = []string{
	PermissionDashboardView,
	PermissionProjectsView,
	PermissionServicesView,
	PermissionServicesManage,
	PermissionServiceTypesView,
	PermissionServiceTypesManage,
	PermissionRolloutTemplatesManage,
	PermissionHostsView,
	PermissionHostsManage,
	PermissionServicesCreate,
	PermissionServicesEdit,
	PermissionServicesDelete,
	PermissionServicesDeploy,
	PermissionServicesStart,
	PermissionServicesStop,
	PermissionServicesRestart,
	PermissionServicesUpdateImage,
	PermissionServicesRemoveContainer,
	PermissionServicesRollback,
	PermissionServicesBatch,
	PermissionServicesEnvironmentCreate,
	PermissionServicesEnvironmentEdit,
	PermissionServicesEnvironmentDelete,
	PermissionServicesRolloutCreate,
	PermissionServicesRolloutPause,
	PermissionServicesRolloutResume,
	PermissionServicesRolloutRetry,
	PermissionServicesAutoCreate,
	PermissionServicesAutoEdit,
	PermissionServicesAutoEnable,
	PermissionServicesAutoDisable,
	PermissionHostsCreate,
	PermissionHostsEdit,
	PermissionHostsTest,
	PermissionHostsDelete,
	PermissionServiceTypesCreate,
	PermissionServiceTypesEdit,
	PermissionServiceTypesDelete,
}

func (u User) HasProject(projectID string) bool {
	if u.Role == RoleAdmin {
		return true
	}
	for _, id := range u.ProjectIDs {
		if id == projectID {
			return true
		}
	}
	return false
}

func (u User) HasPermission(permission string) bool {
	if u.Role == RoleAdmin {
		return true
	}
	module, _, _ := strings.Cut(permission, ".")
	for _, item := range u.Permissions {
		if item == permission {
			return true
		}
		if (module == "services" || module == "hosts" || module == "service_types") && strings.HasPrefix(item, module+".") {
			if permission == module+".view" || item == module+".manage" {
				return true
			}
		}
	}
	return false
}

type RefreshToken struct {
	ID        string    `json:"id" bson:"_id"`
	UserID    string    `json:"user_id" bson:"user_id"`
	TokenHash string    `json:"-" bson:"token_hash"`
	ExpiresAt time.Time `json:"expires_at" bson:"expires_at"`
	Revoked   bool      `json:"revoked" bson:"revoked"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	User         User   `json:"user"`
}

// Host describes one Docker Engine managed by the ops platform.
type Host struct {
	ID string `json:"id" bson:"_id"`
	// ProjectID is retained for records and clients created before hosts could
	// be shared. ProjectIDs is the canonical many-to-many binding.
	ProjectID  string    `json:"project_id" bson:"project_id"`
	ProjectIDs []string  `json:"project_ids,omitempty" bson:"project_ids,omitempty"`
	Name       string    `json:"name" bson:"name"`
	Region     string    `json:"region,omitempty" bson:"region,omitempty"`
	Zone       string    `json:"zone,omitempty" bson:"zone,omitempty"`
	InternalIP string    `json:"internal_ip" bson:"internal_ip"`
	ExternalIP string    `json:"external_ip" bson:"external_ip"`
	DockerHost string    `json:"docker_host" bson:"docker_host"`
	TLSCA      string    `json:"tls_ca" bson:"tls_ca"`
	TLSCert    string    `json:"tls_cert" bson:"tls_cert"`
	TLSKey     string    `json:"tls_key" bson:"tls_key"`
	Note       string    `json:"note" bson:"note"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
}

// HasProject reports whether the host is available to a project. ProjectID
// supports legacy MongoDB documents; new bindings are stored in ProjectIDs.
func (h Host) HasProject(projectID string) bool {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return false
	}
	if strings.TrimSpace(h.ProjectID) == projectID {
		return true
	}
	for _, id := range h.ProjectIDs {
		if strings.TrimSpace(id) == projectID {
			return true
		}
	}
	return false
}

// HasProjectBindings reports whether this host is assigned to any project.
func (h Host) HasProjectBindings() bool {
	if strings.TrimSpace(h.ProjectID) != "" {
		return true
	}
	for _, id := range h.ProjectIDs {
		if strings.TrimSpace(id) != "" {
			return true
		}
	}
	return false
}

// Project groups service types under a logical business project. EnvVars is
// a free-form multi-line text block (e.g. pasted docker run flags or
// KEY=VALUE lines) shared across the project. Service instances copy this
// text as their starting point and can freely edit it afterwards without
// affecting the project's own template.
type Project struct {
	ID        string    `json:"id" bson:"_id"`
	Name      string    `json:"name" bson:"name"`
	Note      string    `json:"note" bson:"note"`
	EnvVars   string    `json:"env_vars" bson:"env_vars"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// Environment is a freely-named deployment environment scoped to a project
// (e.g. "beta", "dev", "dev-1", "dev-2"). It groups service instances the same
// way a project groups environments; it is not bound to a specific Docker
// host, which is chosen per ServiceInstance at deploy time.
type Environment struct {
	ID        string    `json:"id" bson:"_id"`
	ProjectID string    `json:"project_id" bson:"project_id"`
	Name      string    `json:"name" bson:"name"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

type ServiceParam struct {
	Flag  string `json:"flag" bson:"flag"`
	Value string `json:"value" bson:"value"`
}

type ServiceParameterField struct {
	Label        string   `json:"label" bson:"label"`
	Flag         string   `json:"flag" bson:"flag"`
	Kind         string   `json:"kind" bson:"kind"`
	Required     bool     `json:"required" bson:"required"`
	DefaultValue string   `json:"default_value" bson:"default_value"`
	Options      []string `json:"options,omitempty" bson:"options,omitempty"`
}

// ServiceType is a shared deployment template.
type ServiceType struct {
	ParameterFields []ServiceParameterField `json:"parameter_fields,omitempty" bson:"parameter_fields,omitempty"`
	ID              string                  `json:"id" bson:"_id"`
	Name            string                  `json:"name" bson:"name"`
	ReadinessProbe  ReadinessProbe          `json:"readiness_probe" bson:"readiness_probe"`
	CreatedAt       time.Time               `json:"created_at" bson:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at" bson:"updated_at"`
}

// ServiceInstance is a concrete deployable server derived from a ServiceType
// category, e.g. ServiceType "game" can have instances "game-1", "game-2"
// each with their own image, CLI parameters (such as --server-id) and env
// vars. EnvText is a free-form text block (usually copied from the
// project's EnvVars template) that can be freely edited per instance
// without affecting the project or other instances.
//
// ContainerID/Status/StatusText/ContainerImage/StartedAt/DeployError are
// runtime-only fields: they reflect the live Docker container state fetched
// from the instance's host at read time and are never persisted.
type ServiceInstance struct {
	ProjectID        string         `json:"project_id" bson:"project_id"`
	EnvironmentID    string         `json:"environment_id" bson:"environment_id"`
	ID               string         `json:"id" bson:"_id"`
	ServiceTypeID    string         `json:"service_type_id" bson:"service_type_id"`
	HostID           string         `json:"host_id" bson:"host_id"` // Docker host to deploy this instance's container on, chosen at deploy time
	Name             string         `json:"name" bson:"name"`
	RolloutGroup     string         `json:"rollout_group,omitempty" bson:"rollout_group,omitempty"`
	Image            string         `json:"image" bson:"image"`
	Params           []ServiceParam `json:"params" bson:"params"`
	EnvText          string         `json:"env_text" bson:"env_text"`
	Network          string         `json:"network" bson:"network"`           // Docker network name to attach the container to, e.g. "bridge" or a custom user-defined network
	PortMapping      string         `json:"port_mapping" bson:"port_mapping"` // "host_port:container_port", e.g. "9001:9001"; container port defaults to host port if only one number is given
	RestartPolicy    string         `json:"restart_policy" bson:"restart_policy"`
	Note             string         `json:"note" bson:"note"`
	CreatedAt        time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at" bson:"updated_at"`
	AutoOpenPolicyID string         `json:"-" bson:"auto_open_policy_id,omitempty"`

	ContainerID    string `json:"container_id,omitempty" bson:"-"`
	Status         string `json:"status" bson:"-"` // not_deployed | running | stopped | restarting | error | unknown
	StatusText     string `json:"status_text,omitempty" bson:"-"`
	ContainerImage string `json:"container_image,omitempty" bson:"-"`
	StartedAt      string `json:"started_at,omitempty" bson:"-"`
	DeployError    string `json:"deploy_error,omitempty" bson:"-"`
}

// ServiceInstanceStatus enumerates the runtime status values exposed to the
// frontend for a service instance's Docker container.
const (
	StatusNotDeployed = "not_deployed"
	StatusRunning     = "running"
	StatusStopped     = "stopped"
	StatusRestarting  = "restarting"
	StatusError       = "error"
	StatusUnknown     = "unknown"
)
