package internal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gogu-x/ops/admin/internal/application"
	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
)

type trackedServiceOperation struct {
	model.ServiceInstanceOperation
	ownerID string
}

type serviceOperationManager struct {
	mu         sync.RWMutex
	operations map[string]*trackedServiceOperation
	busyIDs    map[string]string
	app        *application.Service
}

var (
	errOperationLimitReached = errors.New("当前批量任务较多，请等待已有任务完成")
	errInstanceOperationBusy = errors.New("所选实例正在执行其他任务")
)

func newServiceOperationManager(app *application.Service) *serviceOperationManager {
	return &serviceOperationManager{operations: make(map[string]*trackedServiceOperation), busyIDs: make(map[string]string), app: app}
}

func (m *serviceOperationManager) start(ownerID, operator string, request model.ServiceInstanceBatchOperationRequest, instances []model.ServiceInstance) (model.ServiceInstanceOperation, error) {
	now := time.Now().UTC()
	operation := model.ServiceInstanceOperation{
		ID:        uuid.NewString(),
		Action:    request.Action,
		Status:    "running",
		Total:     len(instances),
		CreatedAt: now,
		UpdatedAt: now,
		Items:     make([]model.ServiceInstanceOperationItem, len(instances)),
	}
	for index, item := range instances {
		operation.Items[index] = model.ServiceInstanceOperationItem{InstanceID: item.ID, Name: item.Name, Status: "pending"}
	}
	m.mu.Lock()
	m.pruneLocked(now)
	activeOperations := 0
	for _, active := range m.operations {
		if active.Status != "running" {
			continue
		}
		activeOperations++
	}
	if activeOperations >= 10 {
		m.mu.Unlock()
		return model.ServiceInstanceOperation{}, errOperationLimitReached
	}
	for _, item := range instances {
		if _, busy := m.busyIDs[item.ID]; busy {
			m.mu.Unlock()
			return model.ServiceInstanceOperation{}, errInstanceOperationBusy
		}
	}
	m.operations[operation.ID] = &trackedServiceOperation{ServiceInstanceOperation: operation, ownerID: ownerID}
	for _, item := range instances {
		m.busyIDs[item.ID] = operation.ID
	}
	m.mu.Unlock()
	go m.run(operation.ID, operator, request)
	return operation, nil
}

func (m *serviceOperationManager) run(id, operator string, request model.ServiceInstanceBatchOperationRequest) {
	for index := range request.InstanceIDs {
		m.updateItem(id, index, "running", "")
		instanceID := request.InstanceIDs[index]
		var err error
		switch request.Action {
		case "deploy":
			_, err = m.app.DeployInstance(context.Background(), instanceID, operator)
		case "update_image":
			_, err = m.app.UpdateInstanceImage(context.Background(), instanceID, request.Image, operator)
		case "start", "stop", "restart":
			err = m.app.ContainerAction(instanceID, request.Action)
		}
		if err != nil {
			m.updateItem(id, index, "failed", err.Error())
		} else {
			m.updateItem(id, index, "succeeded", "")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	operation := m.operations[id]
	if operation == nil {
		return
	}
	switch {
	case operation.Failed == 0:
		operation.Status = "succeeded"
	case operation.Succeeded == 0:
		operation.Status = "failed"
	default:
		operation.Status = "partial"
	}
	operation.UpdatedAt = time.Now().UTC()
	for _, item := range operation.Items {
		if m.busyIDs[item.InstanceID] == id {
			delete(m.busyIDs, item.InstanceID)
		}
	}
}

func (m *serviceOperationManager) reserve(instanceIDs []string) (func(), error) {
	reservationID := "request:" + uuid.NewString()
	m.mu.Lock()
	for _, id := range instanceIDs {
		if _, busy := m.busyIDs[id]; busy {
			m.mu.Unlock()
			return nil, errInstanceOperationBusy
		}
	}
	for _, id := range instanceIDs {
		m.busyIDs[id] = reservationID
	}
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, id := range instanceIDs {
			if m.busyIDs[id] == reservationID {
				delete(m.busyIDs, id)
			}
		}
	}, nil
}

func (m *serviceOperationManager) updateItem(id string, index int, status, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	operation := m.operations[id]
	if operation == nil || index < 0 || index >= len(operation.Items) {
		return
	}
	item := &operation.Items[index]
	if item.Status == "succeeded" {
		operation.Succeeded--
	} else if item.Status == "failed" {
		operation.Failed--
	}
	item.Status = status
	item.Error = message
	if status == "succeeded" {
		operation.Succeeded++
	} else if status == "failed" {
		operation.Failed++
	}
	if status == "succeeded" || status == "failed" {
		operation.Completed++
	}
	operation.UpdatedAt = time.Now().UTC()
}

func (m *serviceOperationManager) get(id, ownerID string) (model.ServiceInstanceOperation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	operation := m.operations[id]
	if operation == nil || operation.ownerID != ownerID {
		return model.ServiceInstanceOperation{}, false
	}
	result := operation.ServiceInstanceOperation
	result.Items = append([]model.ServiceInstanceOperationItem(nil), operation.Items...)
	return result, true
}

func (m *serviceOperationManager) pruneLocked(now time.Time) {
	for id, operation := range m.operations {
		if operation.Status != "running" && now.Sub(operation.UpdatedAt) > 24*time.Hour {
			delete(m.operations, id)
		}
	}
	if len(m.operations) < 1000 {
		return
	}
	for id, operation := range m.operations {
		if operation.Status != "running" {
			delete(m.operations, id)
		}
	}
}

func supportedBatchAction(action string) bool {
	switch strings.TrimSpace(action) {
	case "deploy", "start", "stop", "restart", "update_image":
		return true
	default:
		return false
	}
}
