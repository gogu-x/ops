package rollout

import (
	"encoding/json"
	"strings"

	"github.com/gogu-x/ops/model"
)

func BuildPlan(request model.CreateRolloutRequest, projectID string, selected, allInstances []model.ServiceInstance, typeByID map[string]model.ServiceType, hostByID map[string]model.Host) ([]model.RolloutGroupPlan, []model.RolloutItem) {
	groupPlans := make(map[string]model.RolloutGroupPlan)
	for _, item := range allInstances {
		typeConfig, ok := typeByID[item.ServiceTypeID]
		if !ok || item.ProjectID != projectID {
			continue
		}
		key, name := rolloutGroup(item, typeConfig, hostByID[item.HostID])
		plan, exists := groupPlans[key]
		if !exists {
			plan = model.RolloutGroupPlan{Key: key, Name: name, ProjectID: projectID, MaxUnavailable: request.MaxUnavailablePerGroup, MaxParallel: request.MaxParallelPerGroup}
		}
		if item.Status != model.StatusRunning {
			plan.InitialUnavailable++
		}
		groupPlans[key] = plan
	}
	groups := make([]model.RolloutGroupPlan, 0, len(groupPlans))
	for _, group := range groupPlans {
		groups = append(groups, group)
	}
	items := make([]model.RolloutItem, 0, len(selected))
	var values map[string]string
	if len(request.ParameterValues) > 0 {
		values = request.ParameterValues
	}
	for _, item := range selected {
		typeConfig := typeByID[item.ServiceTypeID]
		key, _ := rolloutGroup(item, typeConfig, hostByID[item.HostID])
		target := item.Image
		if request.Action == "update_image" {
			target = request.Image
		}
		items = append(items, model.RolloutItem{
			InstanceID:          item.ID,
			Name:                item.Name,
			HostID:              item.HostID,
			PreviousContainerID: item.ContainerID,
			GroupKey:            key,
			PreviousImage:       item.Image,
			TargetImage:         target,
			ParameterValues:     values,
			InitiallyAvailable:  item.Status == model.StatusRunning,
		})
	}
	return groups, items
}

func rolloutGroup(instance model.ServiceInstance, serviceType model.ServiceType, host model.Host) (string, string) {
	region := strings.TrimSpace(host.Region)
	if region == "" {
		region = "未标注机房"
	}
	zone := strings.TrimSpace(host.Zone)
	if zone == "" {
		zone = instance.HostID
	}
	group := strings.TrimSpace(instance.RolloutGroup)
	if group == "" {
		group = serviceType.Name
	}
	keyParts := []string{instance.ProjectID, instance.EnvironmentID, serviceType.ID, region, zone, group}
	encodedKey, _ := json.Marshal(keyParts)
	key := string(encodedKey)
	zoneName := host.Zone
	if strings.TrimSpace(zoneName) == "" {
		zoneName = host.Name
	}
	name := strings.Join([]string{region, zoneName, serviceType.Name, group}, " / ")
	return key, name
}

func HasSingleServiceType(instances []model.ServiceInstance) bool {
	if len(instances) == 0 {
		return false
	}
	serviceTypeID := instances[0].ServiceTypeID
	for _, instance := range instances[1:] {
		if instance.ServiceTypeID != serviceTypeID {
			return false
		}
	}
	return true
}
