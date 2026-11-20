package model

import "time"

type AutoOpenTemplate struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	ProjectID     string                  `json:"project_id"`
	EnvironmentID string                  `json:"environment_id"`
	Image         string                  `json:"image"`
	Parameters    []AutoOpenTemplateParam `json:"parameters"`
}

type AutoOpenTemplateParam struct {
	Flag         string `json:"flag"`
	Configured   bool   `json:"configured"`
	DisplayValue string `json:"display_value,omitempty"`
}

type AutoOpenSettings struct {
	Name          string    `json:"name" bson:"name"`
	TemplateID    string    `json:"template_id" bson:"template_id"`
	NamePrefix    string    `json:"name_prefix" bson:"name_prefix"`
	FirstServerID int       `json:"first_server_id" bson:"first_server_id"`
	FirstOpenAt   time.Time `json:"first_open_at" bson:"first_open_at"`
	IntervalDays  int       `json:"interval_days" bson:"interval_days"`
	Count         int       `json:"count" bson:"count"`
	FirstHostPort int       `json:"first_host_port" bson:"first_host_port"`
}

type AutoOpenRun struct {
	ServerID     int       `json:"server_id" bson:"server_id"`
	InstanceID   string    `json:"instance_id" bson:"instance_id"`
	InstanceName string    `json:"instance_name" bson:"instance_name"`
	OpenAt       time.Time `json:"open_at" bson:"open_at"`
	FinishedAt   time.Time `json:"finished_at" bson:"finished_at"`
	Error        string    `json:"error,omitempty" bson:"error,omitempty"`
}

type AutoOpenPolicy struct {
	AutoOpenSettings `bson:",inline"`
	ID               string        `json:"id" bson:"_id"`
	ProjectID        string        `json:"project_id" bson:"project_id"`
	EnvironmentID    string        `json:"environment_id" bson:"environment_id"`
	TemplateName     string        `json:"template_name" bson:"template_name"`
	Enabled          bool          `json:"enabled" bson:"enabled"`
	Completed        int           `json:"completed" bson:"completed"`
	NextOpenAt       time.Time     `json:"next_open_at" bson:"next_open_at"`
	Running          bool          `json:"running" bson:"running"`
	RecentRuns       []AutoOpenRun `json:"recent_runs" bson:"recent_runs"`
	UpdatedAt        time.Time     `json:"updated_at" bson:"updated_at"`
	OperatorID       string        `json:"-" bson:"operator_id"`
	LeaseUntil       time.Time     `json:"-" bson:"lease_until"`
	Revision         int64         `json:"revision" bson:"revision"`
}
