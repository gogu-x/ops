package model

import "time"

type User struct {
	ID           string    `json:"id" bson:"_id"`
	Username     string    `json:"username" bson:"username"`
	PasswordHash string    `json:"-" bson:"password_hash"`
	Role         string    `json:"role" bson:"role"`
	Disabled     bool      `json:"disabled" bson:"disabled"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
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
	ID         string    `json:"id" bson:"_id"`
	Name       string    `json:"name" bson:"name"`
	DockerHost string    `json:"docker_host" bson:"docker_host"`
	TLSCA      string    `json:"tls_ca" bson:"tls_ca"`
	TLSCert    string    `json:"tls_cert" bson:"tls_cert"`
	TLSKey     string    `json:"tls_key" bson:"tls_key"`
	Note       string    `json:"note" bson:"note"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
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

// ServiceType describes a configurable Docker service and its CLI parameters.
type ServiceParam struct {
	Flag  string `json:"flag" bson:"flag"`
	Value string `json:"value" bson:"value"`
}

// ServiceType is a lightweight category (e.g. "game", "battle") scoped to a
// project + host. The actual image, CLI params, env vars and notes are
// configured per deployable ServiceInstance, not on the type itself.
type ServiceType struct {
	ID        string    `json:"id" bson:"_id"`
	ProjectID string    `json:"project_id" bson:"project_id"`
	HostID    string    `json:"host_id" bson:"host_id"`
	Name      string    `json:"name" bson:"name"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
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
	ID            string         `json:"id" bson:"_id"`
	ServiceTypeID string         `json:"service_type_id" bson:"service_type_id"`
	Name          string         `json:"name" bson:"name"`
	Image         string         `json:"image" bson:"image"`
	Params        []ServiceParam `json:"params" bson:"params"`
	EnvText       string         `json:"env_text" bson:"env_text"`
	Network       string         `json:"network" bson:"network"`           // Docker network name to attach the container to, e.g. "bridge" or a custom user-defined network
	PortMapping   string         `json:"port_mapping" bson:"port_mapping"` // "host_port:container_port", e.g. "9001:9001"; container port defaults to host port if only one number is given
	Note          string         `json:"note" bson:"note"`
	CreatedAt     time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" bson:"updated_at"`

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
