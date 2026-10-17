package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/gogu-x/ops/model"
)

func (m *DockerManager) ContainerResourceStats(ctx context.Context, h model.Host, containerID string) (model.ContainerResourceMetrics, error) {
	cli, err := m.Client(h)
	if err != nil {
		return model.ContainerResourceMetrics{}, err
	}
	reader, err := cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return model.ContainerResourceMetrics{}, err
	}
	defer reader.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(reader.Body).Decode(&stats); err != nil {
		return model.ContainerResourceMetrics{}, fmt.Errorf("decode container resource stats: %w", err)
	}
	return projectContainerResourceStats(stats), nil
}

func projectContainerResourceStats(stats container.StatsResponse) model.ContainerResourceMetrics {
	metrics := model.ContainerResourceMetrics{Processes: stats.PidsStats.Current}
	if stats.Read.IsZero() {
		metrics.SampledAt = time.Now().UTC()
	} else {
		metrics.SampledAt = stats.Read.UTC()
	}

	cpuDelta := difference(stats.CPUStats.CPUUsage.TotalUsage, stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := difference(stats.CPUStats.SystemUsage, stats.PreCPUStats.SystemUsage)
	cpuCount := uint64(stats.CPUStats.OnlineCPUs)
	if cpuCount == 0 {
		cpuCount = uint64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta > 0 && cpuCount > 0 {
		metrics.CPUPercent = float64(cpuDelta) / float64(systemDelta) * float64(cpuCount) * 100
	}

	memoryUsage := stats.MemoryStats.Usage
	cacheKey := "cache"
	if inactiveFile, exists := stats.MemoryStats.Stats["inactive_file"]; exists {
		cacheKey = "inactive_file"
		memoryUsage = subtractFloor(memoryUsage, inactiveFile)
	} else if cache, exists := stats.MemoryStats.Stats[cacheKey]; exists {
		memoryUsage = subtractFloor(memoryUsage, cache)
	}
	metrics.MemoryUsage = memoryUsage
	metrics.MemoryLimit = stats.MemoryStats.Limit
	if metrics.MemoryLimit > 0 {
		metrics.MemoryPercent = float64(metrics.MemoryUsage) / float64(metrics.MemoryLimit) * 100
	}

	for _, network := range stats.Networks {
		metrics.NetworkRXBytes += network.RxBytes
		metrics.NetworkTXBytes += network.TxBytes
	}
	for _, entry := range stats.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(entry.Op) {
		case "read":
			metrics.BlockReadBytes += entry.Value
		case "write":
			metrics.BlockWriteBytes += entry.Value
		}
	}
	return metrics
}

func difference(current, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}

func subtractFloor(value, subtract uint64) uint64 {
	if subtract >= value {
		return 0
	}
	return value - subtract
}
