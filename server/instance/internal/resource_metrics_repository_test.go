package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gogu-x/ops/model"
)

func TestMemoryResourceMetricsRepositoryKeepsLatestSamplesPerInstance(t *testing.T) {
	repository := NewMemoryResourceMetricsRepository()
	base := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	for index := 0; index <= resourceHistoryLimit; index++ {
		if err := repository.Create(context.Background(), model.ContainerResourceSample{
			ID:         fmt.Sprintf("sample-%03d", index),
			InstanceID: "service-a",
			SampledAt:  base.Add(time.Duration(index) * time.Second),
			Metrics:    model.ContainerResourceMetrics{CPUPercent: float64(index)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.Create(context.Background(), model.ContainerResourceSample{
		ID: "service-b-sample", InstanceID: "service-b", SampledAt: base,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := repository.ListByInstance(context.Background(), "service-a", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Metrics.CPUPercent != float64(resourceHistoryLimit-1) || got[1].Metrics.CPUPercent != float64(resourceHistoryLimit) {
		t.Fatalf("got latest samples %+v, want the last two samples in chronological order", got)
	}
	other, err := repository.ListByInstance(context.Background(), "service-b", 0)
	if err != nil || len(other) != 1 || other[0].ID != "service-b-sample" {
		t.Fatalf("other instance history = %+v, err = %v", other, err)
	}
}

func TestResourceHistoryReturnsSavedSamplesWithoutFetchingDocker(t *testing.T) {
	instances := NewMemoryRepository()
	if err := instances.Create(context.Background(), model.ServiceInstance{ID: "service-a"}); err != nil {
		t.Fatal(err)
	}
	metrics := NewMemoryResourceMetricsRepository()
	sampledAt := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	if err := metrics.Create(context.Background(), model.ContainerResourceSample{
		ID: "sample-a", InstanceID: "service-a", SampledAt: sampledAt,
		Metrics: model.ContainerResourceMetrics{CPUPercent: 15, SampledAt: sampledAt},
	}); err != nil {
		t.Fatal(err)
	}
	service := NewInstanceServiceWithMonitoring(instances, nil, nil, metrics)

	got, err := service.resourceHistory("service-a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Latest == nil || got.Latest.CPUPercent != 15 || len(got.Samples) != 1 || !got.Latest.SampledAt.Equal(sampledAt) {
		t.Fatalf("resource history = %+v, want the persisted sample", got)
	}
}
