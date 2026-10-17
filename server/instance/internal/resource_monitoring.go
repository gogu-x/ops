package internal

import (
	"context"
	"errors"

	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
)

const defaultResourceHistoryLimit = 60

func (is *InstanceService) resourceHistory(id string, limit int) (model.InstanceResourceHistoryResponse, error) {
	if _, err := is.repo.Get(bgCtx(), id); err != nil {
		return model.InstanceResourceHistoryResponse{}, err
	}
	if limit <= 0 {
		limit = defaultResourceHistoryLimit
	}
	if limit > resourceHistoryLimit {
		limit = resourceHistoryLimit
	}
	samples, err := is.resourceMetrics.ListByInstance(context.Background(), id, limit)
	if err != nil {
		return model.InstanceResourceHistoryResponse{}, err
	}
	response := model.InstanceResourceHistoryResponse{Samples: make([]model.ContainerResourceMetrics, 0, len(samples))}
	for _, sample := range samples {
		response.Samples = append(response.Samples, sample.Metrics)
	}
	if len(response.Samples) > 0 {
		latest := response.Samples[len(response.Samples)-1]
		response.Latest = &latest
	}
	return response, nil
}

func (is *InstanceService) collectResourceStats(ctx context.Context, instanceID, hostID, containerID string) error {
	value, err := hostRequest(model.ContainerResourceStatsRequest{HostID: hostID, ContainerID: containerID})
	if err != nil {
		return err
	}
	result, ok := value.(model.ContainerResourceStatsResponse)
	if !ok {
		return errors.New("invalid host actor response")
	}
	metrics := result.Metrics
	if metrics.SampledAt.IsZero() {
		return errors.New("host returned a resource sample without a timestamp")
	}
	return is.resourceMetrics.Create(ctx, model.ContainerResourceSample{
		ID:         uuid.NewString(),
		InstanceID: instanceID,
		SampledAt:  metrics.SampledAt,
		Metrics:    metrics,
	})
}
