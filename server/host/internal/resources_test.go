package internal

import (
	"math"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

func TestProjectContainerResourceStats(t *testing.T) {
	readAt := time.Date(2026, 10, 17, 8, 30, 0, 0, time.UTC)
	stats := container.StatsResponse{
		Stats: container.Stats{
			Read: readAt,
			CPUStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 2_000},
				SystemUsage: 10_000,
				OnlineCPUs:  4,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 1_000},
				SystemUsage: 5_000,
			},
			MemoryStats: container.MemoryStats{
				Usage: 800,
				Limit: 2_000,
				Stats: map[string]uint64{"inactive_file": 200},
			},
			PidsStats: container.PidsStats{Current: 7},
			BlkioStats: container.BlkioStats{IoServiceBytesRecursive: []container.BlkioStatEntry{
				{Op: "read", Value: 1_024},
				{Op: "WRITE", Value: 2_048},
				{Op: "sync", Value: 4_096},
			}},
		},
		Networks: map[string]container.NetworkStats{
			"app": {RxBytes: 10, TxBytes: 20},
			"db":  {RxBytes: 30, TxBytes: 40},
		},
	}

	got := projectContainerResourceStats(stats)
	if math.Abs(got.CPUPercent-80) > 0.001 {
		t.Errorf("CPUPercent = %v, want 80", got.CPUPercent)
	}
	if got.MemoryUsage != 600 || got.MemoryLimit != 2_000 || math.Abs(got.MemoryPercent-30) > 0.001 {
		t.Errorf("memory metrics = (%d, %d, %v), want (600, 2000, 30)", got.MemoryUsage, got.MemoryLimit, got.MemoryPercent)
	}
	if got.NetworkRXBytes != 40 || got.NetworkTXBytes != 60 {
		t.Errorf("network metrics = (%d, %d), want (40, 60)", got.NetworkRXBytes, got.NetworkTXBytes)
	}
	if got.BlockReadBytes != 1_024 || got.BlockWriteBytes != 2_048 || got.Processes != 7 {
		t.Errorf("block/process metrics = (%d, %d, %d)", got.BlockReadBytes, got.BlockWriteBytes, got.Processes)
	}
	if !got.SampledAt.Equal(readAt) {
		t.Errorf("SampledAt = %s, want %s", got.SampledAt, readAt)
	}
}

func TestProjectContainerResourceStatsHandlesMissingDeltasAndUnderflow(t *testing.T) {
	got := projectContainerResourceStats(container.StatsResponse{
		Stats: container.Stats{
			CPUStats:    container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 1}, OnlineCPUs: 1},
			PreCPUStats: container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 2}},
			MemoryStats: container.MemoryStats{Usage: 10, Limit: 10, Stats: map[string]uint64{"cache": 20}},
		},
	})
	if got.CPUPercent != 0 || got.MemoryUsage != 0 || got.MemoryPercent != 0 {
		t.Errorf("metrics = %+v, want zero CPU and clamped memory values", got)
	}
}
