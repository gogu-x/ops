package model

import (
	"time"

	"github.com/docker/docker/api/types"
)

type ListRequest struct{}
type CreateRequest struct{ Host Host }
type DeleteRequest struct{ ID string }
type TestRequest struct{ ID string }
type HostListResponse struct{ Hosts []Host }
type TestResponse struct {
	Host   Host              `json:"host"`
	Docker map[string]string `json:"docker"`
}

// ContainerListRequest lists ops-managed containers on a given host.
type ContainerListRequest struct{ HostID string }
type ContainerListResponse struct{ Containers []types.Container }

// ContainerInspectRequest fetches full details for a single container on a host.
type ContainerInspectRequest struct {
	HostID      string
	ContainerID string
}
type ContainerInspectResponse struct{ Container types.ContainerJSON }

// ContainerDeployRequest creates and starts a new container on a host.
type ContainerDeployRequest struct {
	HostID string
	Spec   ContainerSpec
}
type ContainerDeployResponse struct{ ContainerID string }

// ContainerActionRequest performs start/stop/restart/remove on an existing container.
type ContainerActionRequest struct {
	HostID      string
	ContainerID string
	Action      string // "start", "stop", "restart", "remove"
}

// ContainerLogsRequest fetches recent log output for a container.
type ContainerLogsRequest struct {
	HostID      string
	ContainerID string
	Tail        string
}
type ContainerLogsResponse struct{ Logs string }

// ImagePullRequest pulls an image onto a host from its registry.
type ImagePullRequest struct {
	HostID string
	Image  string
}

type ListResponse struct{ ServiceInstances []ServiceInstance }
type EnrichRequest struct{ ServiceInstances []ServiceInstance }

type DeployRequest struct{ ID string }

// UpdateImageRequest updates a service instance's image (e.g. to a new
// version tag), pulls the new image on the instance's host, and redeploys
// the container so it runs the updated image.
type UpdateImageRequest struct {
	ID    string
	Image string
}

// InstanceContainerActionRequest performs start/stop/restart/remove on the
// container backing a service instance, identified by instance ID rather
// than host ID + container ID (see ContainerActionRequest for the
// host-scoped equivalent used by the host actor).
type InstanceContainerActionRequest struct {
	ID     string
	Action string
}

// RemoveContainerRequest stops (if needed) and removes the container behind
// a service instance, without deleting the instance's metadata.
type RemoveContainerRequest struct{ ID string }

// StatusRequest returns the live container status for a single instance.
type StatusRequest struct{ ID string }

// DetailRequest returns full Docker inspect information for a deployed
// service instance's container.
type DetailRequest struct{ ID string }

// ContainerDetail is a JSON-friendly projection of the Docker inspect
// response, exposing the fields useful for troubleshooting in the UI.
type ContainerDetail struct {
	ContainerID   string            `json:"container_id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	Status        string            `json:"status"`
	State         string            `json:"state"`
	Running       bool              `json:"running"`
	RestartCount  int               `json:"restart_count"`
	StartedAt     string            `json:"started_at"`
	FinishedAt    string            `json:"finished_at"`
	ExitCode      int               `json:"exit_code"`
	Error         string            `json:"error,omitempty"`
	Cmd           []string          `json:"cmd"`
	Env           []string          `json:"env"`
	Labels        map[string]string `json:"labels"`
	NetworkMode   string            `json:"network_mode"`
	RestartPolicy string            `json:"restart_policy"`
	IPAddress     string            `json:"ip_address"`
	Ports         []string          `json:"ports"`
	Mounts        []string          `json:"mounts"`
	Created       string            `json:"created"`
}
type DetailResponse struct{ Detail ContainerDetail }

// LogsRequest returns recent log output for a service instance's container.
type LogsRequest struct {
	ID   string
	Tail string
}
type LogsResponse struct{ Logs string }

// InstanceEvent records a notable occurrence in a service instance's
// lifecycle (deploy, start/stop/restart, image update, health check),
// shown as a timeline in the ops UI's instance detail panel.
type InstanceEvent struct {
	ID         string    `json:"id" bson:"_id"`
	InstanceID string    `json:"instance_id" bson:"instance_id"`
	Type       string    `json:"type" bson:"type"`
	Message    string    `json:"message" bson:"message"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
}

// InstanceEvent type constants.
const (
	EventDeploySucceeded        = "deploy_succeeded"
	EventDeployFailed           = "deploy_failed"
	EventUpdateImageSucceeded   = "update_image_succeeded"
	EventUpdateImageFailed      = "update_image_failed"
	EventContainerStarted       = "container_started"
	EventContainerStartFailed   = "container_start_failed"
	EventContainerStopped       = "container_stopped"
	EventContainerStopFailed    = "container_stop_failed"
	EventContainerRestarted     = "container_restarted"
	EventContainerRestartFailed = "container_restart_failed"
	EventContainerRemoved       = "container_removed"
	EventContainerRemoveFailed  = "container_remove_failed"
	EventHealthCheckPassed      = "health_check_passed"
	EventHealthCheckFailed      = "health_check_failed"
)

// ListEventsRequest asks the ops-instance actor for an instance's recent
// events. Limit <= 0 means no limit.
type ListEventsRequest struct {
	ID    string
	Limit int
}
type EventsResponse struct{ Events []InstanceEvent }
