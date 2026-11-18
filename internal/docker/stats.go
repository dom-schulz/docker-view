package docker

import (
	"context"
	"encoding/json"

	"github.com/docker/docker/api/types/container"
)

// Stats is one CPU and memory sample for a container.
type Stats struct {
	ContainerID string
	CPUPercent  float64
	MemUsage    uint64
	MemLimit    uint64
}

// MemPercent is memory use as a share of the limit.
func (s Stats) MemPercent() float64 {
	if s.MemLimit == 0 {
		return 0
	}
	return float64(s.MemUsage) / float64(s.MemLimit) * 100
}

// StreamStats sends a sample about once a second until the container stops,
// ctx is cancelled, or send returns false.
func (c *Client) StreamStats(ctx context.Context, containerID string, send func(Stats) bool) error {
	resp, err := c.api.ContainerStats(ctx, containerID, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	go func() { <-ctx.Done(); resp.Body.Close() }()

	dec := json.NewDecoder(resp.Body)
	for {
		var v container.StatsResponse
		if err := dec.Decode(&v); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if !send(ComputeStats(containerID, v)) {
			return nil
		}
	}
}

// ComputeStats turns a raw stats response into percentages the way
// `docker stats` does.
func ComputeStats(id string, v container.StatsResponse) Stats {
	s := Stats{ContainerID: id, MemLimit: v.MemoryStats.Limit}

	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	online := float64(v.CPUStats.OnlineCPUs)
	if online == 0 {
		online = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpuDelta > 0 && sysDelta > 0 {
		s.CPUPercent = cpuDelta / sysDelta * online * 100
	}

	// Page cache isn't real usage: cgroup v2 reports it as inactive_file,
	// cgroup v1 as total_inactive_file.
	usage := v.MemoryStats.Usage
	cache := v.MemoryStats.Stats["inactive_file"]
	if c, ok := v.MemoryStats.Stats["total_inactive_file"]; ok {
		cache = c
	}
	if cache < usage {
		usage -= cache
	}
	s.MemUsage = usage
	return s
}
