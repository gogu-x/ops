package internal

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gogu-x/ops/model"
	"github.com/gogu-x/tree"
	treetimer "github.com/gogu-x/tree/timer"
)

const (
	resourceCollectionInterval     = 30 * time.Second
	resourceCollectionStartupDelay = 5 * time.Second
	resourceCollectionWorkers      = 8
)

const resourceCollectionTimerType treetimer.TimerType = 1

type resourceCollectionTarget struct {
	instanceID  string
	hostID      string
	containerID string
}

func (is *InstanceService) startResourceCollector(actorCtx tree.Context) {
	is.resourceSystem = actorCtx.System()
	is.resourceSelf = actorCtx.Self()
	wheel := treetimer.NewTimeWheel(1, is.resourceSelf, is.resourceSystem)
	is.resourceTimer = wheel
	wheel.Register(resourceCollectionTimerType, is.handleResourceCollectionTimer)

	if wheel.After(resourceCollectionTimerType, resourceCollectionStartupDelay, nil) == nil {
		log.Printf("[ops] schedule initial resource collection")
	}
}

func (is *InstanceService) handleResourceCollectionTimer(_ interface{}) {
	collectionCtx := is.collectorCtx
	if collectionCtx == nil || collectionCtx.Err() != nil {
		return
	}

	go is.runResourceCollection(collectionCtx)
}

func (is *InstanceService) runResourceCollection(ctx context.Context) {
	is.collectResourceSamples(ctx)
	// Re-arm from the actor goroutine after collection completes, so slow
	// collection runs cannot overlap and timer operations stay actor-bound.
	is.resourceSystem.SendCallback(is.resourceSelf, is.handleResourceCollectionComplete, nil, nil)
}

func (is *InstanceService) handleResourceCollectionComplete(_ tree.Context, _ interface{}, _ error) {
	if is.collectorCtx != nil && is.collectorCtx.Err() == nil {
		is.resourceTimer.After(resourceCollectionTimerType, resourceCollectionInterval, nil)
	}
}

func (is *InstanceService) collectResourceSamples(ctx context.Context) {
	instances, err := is.repo.List(ctx)
	if err != nil {
		log.Printf("[ops] list services for resource collection: %v", err)
		return
	}
	byHost := make(map[string][]model.ServiceInstance)
	for _, item := range instances {
		if item.HostID != "" {
			byHost[item.HostID] = append(byHost[item.HostID], item)
		}
	}

	targets := make([]resourceCollectionTarget, 0, len(instances))
	failedHosts := 0
	for hostID, items := range byHost {
		if ctx.Err() != nil {
			return
		}
		value, err := hostRequest(model.ContainerListRequest{HostID: hostID})
		if err != nil {
			failedHosts++
			continue
		}
		response, ok := value.(model.ContainerListResponse)
		if !ok {
			failedHosts++
			continue
		}
		for _, item := range items {
			container, found := findContainerForInstance(response.Containers, item.ID)
			if found && container.State == "running" {
				targets = append(targets, resourceCollectionTarget{instanceID: item.ID, hostID: hostID, containerID: container.ID})
			}
		}
	}

	if len(targets) == 0 {
		if failedHosts > 0 {
			log.Printf("[ops] resource collection skipped: %d host(s) unavailable", failedHosts)
		}
		return
	}

	workerCount := min(resourceCollectionWorkers, len(targets))
	jobs := make(chan resourceCollectionTarget)
	var workers sync.WaitGroup
	var resultMu sync.Mutex
	sampled, failed := 0, 0
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for target := range jobs {
				if ctx.Err() != nil {
					continue
				}
				if err := is.collectResourceStats(ctx, target.instanceID, target.hostID, target.containerID); err != nil {
					resultMu.Lock()
					failed++
					resultMu.Unlock()
					continue
				}
				resultMu.Lock()
				sampled++
				resultMu.Unlock()
			}
		}()
	}

sendTargets:
	for _, target := range targets {
		select {
		case <-ctx.Done():
			break sendTargets
		case jobs <- target:
		}
	}
	close(jobs)
	workers.Wait()
	if failedHosts > 0 || failed > 0 {
		log.Printf("[ops] resource collection completed with errors: sampled=%d failed=%d unavailable_hosts=%d", sampled, failed, failedHosts)
	}
}
