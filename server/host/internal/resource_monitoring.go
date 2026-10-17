package internal

import (
	"context"
	"time"

	"github.com/gogu-x/ops/model"
)

func (a *DockerService) containerResourceStats(hostID, containerID string) (model.ContainerResourceStatsResponse, error) {
	host, err := a.repo.Get(context.Background(), hostID)
	if err != nil {
		return model.ContainerResourceStatsResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	metrics, err := a.docker.ContainerResourceStats(ctx, host, containerID)
	if err != nil {
		return model.ContainerResourceStatsResponse{}, err
	}
	return model.ContainerResourceStatsResponse{Metrics: metrics}, nil
}
