package model

import "time"

// ReadinessProbe describes the check that gates a rolling deployment wave.
// TCP and HTTP ports are host-published ports reachable from the Ops API.
type ReadinessProbe struct {
	Type string `json:"type" bson:"type"` // "docker", "tcp", or "http"
	Port int    `json:"port,omitempty" bson:"port,omitempty"`
	Path string `json:"path,omitempty" bson:"path,omitempty"`
}

type CreateRolloutRequest struct {
	ServiceParameterValuesRequest
	ReadinessProbe         *ReadinessProbe `json:"readiness_probe,omitempty"`
	InstanceIDs            []string        `json:"instance_ids" binding:"required,min=1"`
	Action                 string          `json:"action" binding:"required"` // restart | update_image
	Image                  string          `json:"image,omitempty"`
	MaxUnavailablePerGroup int             `json:"max_unavailable_per_group"`
	MaxParallel            int             `json:"max_parallel"`
	MaxParallelPerGroup    int             `json:"max_parallel_per_group"`
	HealthTimeoutSeconds   int             `json:"health_timeout_seconds"`
}

type RolloutGroupPlan struct {
	Key                string `json:"key" bson:"key"`
	Name               string `json:"name" bson:"name"`
	ProjectID          string `json:"project_id" bson:"project_id"`
	InitialUnavailable int    `json:"initial_unavailable" bson:"initial_unavailable"`
	MaxUnavailable     int    `json:"max_unavailable" bson:"max_unavailable"`
	MaxParallel        int    `json:"max_parallel" bson:"max_parallel"`
}

type RolloutJob struct {
	ID                     string             `json:"id" bson:"_id"`
	ProjectID              string             `json:"project_id" bson:"project_id"`
	OwnerID                string             `json:"owner_id" bson:"owner_id"`
	Operator               string             `json:"operator" bson:"operator"`
	Action                 string             `json:"action" bson:"action"`
	Image                  string             `json:"image,omitempty" bson:"image,omitempty"`
	Status                 string             `json:"status" bson:"status"` // queued | running | paused | succeeded | partial | failed
	PauseReason            string             `json:"pause_reason,omitempty" bson:"pause_reason,omitempty"`
	Total                  int                `json:"total" bson:"total"`
	Completed              int                `json:"completed" bson:"completed"`
	Succeeded              int                `json:"succeeded" bson:"succeeded"`
	Failed                 int                `json:"failed" bson:"failed"`
	MaxUnavailablePerGroup int                `json:"max_unavailable_per_group" bson:"max_unavailable_per_group"`
	MaxParallel            int                `json:"max_parallel" bson:"max_parallel"`
	MaxParallelPerGroup    int                `json:"max_parallel_per_group" bson:"max_parallel_per_group"`
	HealthTimeoutSeconds   int                `json:"health_timeout_seconds" bson:"health_timeout_seconds"`
	Groups                 []RolloutGroupPlan `json:"groups" bson:"groups"`
	LeaseOwner             string             `json:"-" bson:"lease_owner,omitempty"`
	LeaseUntil             time.Time          `json:"-" bson:"lease_until,omitempty"`
	CreatedAt              time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt              time.Time          `json:"updated_at" bson:"updated_at"`
}

type RolloutItem struct {
	ParameterValues     map[string]string `json:"-" bson:"parameter_values,omitempty"`
	ID                  string            `json:"id" bson:"_id"`
	JobID               string            `json:"job_id" bson:"job_id"`
	InstanceID          string            `json:"instance_id" bson:"instance_id"`
	Name                string            `json:"name" bson:"name"`
	HostID              string            `json:"host_id" bson:"host_id"`
	PreviousContainerID string            `json:"previous_container_id,omitempty" bson:"previous_container_id,omitempty"`
	GroupKey            string            `json:"group_key" bson:"group_key"`
	PreviousImage       string            `json:"previous_image" bson:"previous_image"`
	TargetImage         string            `json:"target_image" bson:"target_image"`
	ReadinessProbe      ReadinessProbe    `json:"readiness_probe" bson:"readiness_probe"`
	InitiallyAvailable  bool              `json:"initially_available" bson:"initially_available"`
	Status              string            `json:"status" bson:"status"` // pending | running | health_check | succeeded | failed
	Attempt             int               `json:"attempt" bson:"attempt"`
	Error               string            `json:"error,omitempty" bson:"error,omitempty"`
	StartedAt           time.Time         `json:"started_at,omitempty" bson:"started_at,omitempty"`
	UpdatedAt           time.Time         `json:"updated_at" bson:"updated_at"`
	FinishedAt          time.Time         `json:"finished_at,omitempty" bson:"finished_at,omitempty"`
}

type RolloutAuditEvent struct {
	ID         string    `json:"id" bson:"_id"`
	JobID      string    `json:"job_id" bson:"job_id"`
	InstanceID string    `json:"instance_id,omitempty" bson:"instance_id,omitempty"`
	Operator   string    `json:"operator" bson:"operator"`
	Type       string    `json:"type" bson:"type"`
	Message    string    `json:"message" bson:"message"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
}

type RolloutDetail struct {
	Job    RolloutJob          `json:"job"`
	Items  []RolloutItem       `json:"items"`
	Events []RolloutAuditEvent `json:"events"`
}

type RolloutTemplate struct {
	ID                  string          `json:"id" bson:"_id"`
	Kind                string          `json:"kind" bson:"kind"`
	Name                string          `json:"name" bson:"name"`
	MaxUnavailable      int             `json:"maxUnavailable,omitempty" bson:"max_unavailable,omitempty"`
	MaxParallel         int             `json:"maxParallel,omitempty" bson:"max_parallel,omitempty"`
	MaxParallelPerGroup int             `json:"maxParallelPerGroup,omitempty" bson:"max_parallel_per_group,omitempty"`
	Probe               *ReadinessProbe `json:"probe,omitempty" bson:"probe,omitempty"`
	Timeout             int             `json:"timeout,omitempty" bson:"timeout,omitempty"`
}
