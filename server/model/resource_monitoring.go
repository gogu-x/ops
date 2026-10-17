package model

import "time"

// ContainerResourceMetrics is a point-in-time projection of Docker container
// statistics. CPU and memory percentages are expressed from 0 to 100, while
// byte counters are cumulative values reported by Docker.
type ContainerResourceMetrics struct {
	CPUPercent      float64   `json:"cpu_percent"`
	MemoryUsage     uint64    `json:"memory_usage"`
	MemoryLimit     uint64    `json:"memory_limit"`
	MemoryPercent   float64   `json:"memory_percent"`
	NetworkRXBytes  uint64    `json:"network_rx_bytes"`
	NetworkTXBytes  uint64    `json:"network_tx_bytes"`
	BlockReadBytes  uint64    `json:"block_read_bytes"`
	BlockWriteBytes uint64    `json:"block_write_bytes"`
	Processes       uint64    `json:"processes"`
	SampledAt       time.Time `json:"sampled_at"`
}

// ContainerResourceStatsRequest requests a one-shot stats sample from a host.
type ContainerResourceStatsRequest struct {
	HostID      string
	ContainerID string
}

type ContainerResourceStatsResponse struct {
	Metrics ContainerResourceMetrics
}

// ContainerResourceSample is a persisted sample collected by the backend.
type ContainerResourceSample struct {
	ID         string                   `json:"id" bson:"_id"`
	InstanceID string                   `json:"instance_id" bson:"instance_id"`
	SampledAt  time.Time                `json:"sampled_at" bson:"sampled_at"`
	Metrics    ContainerResourceMetrics `json:"metrics" bson:"metrics"`
}

// InstanceResourceHistoryRequest reads previously collected samples.
type InstanceResourceHistoryRequest struct {
	ID    string
	Limit int
}

type InstanceResourceHistoryResponse struct {
	Latest  *ContainerResourceMetrics  `json:"latest"`
	Samples []ContainerResourceMetrics `json:"samples"`
}
