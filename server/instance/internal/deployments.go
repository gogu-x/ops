package internal

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// DeploymentRepository stores versioned deployment attempts for an instance.
type DeploymentRepository interface {
	Create(ctx context.Context, deployment model.InstanceDeployment) error
	ListByInstance(ctx context.Context, instanceID string, limit int) ([]model.InstanceDeployment, error)
}

type MemoryDeploymentRepository struct {
	mu    sync.RWMutex
	items []model.InstanceDeployment
}

func NewMemoryDeploymentRepository() *MemoryDeploymentRepository {
	return &MemoryDeploymentRepository{}
}

func (r *MemoryDeploymentRepository) Create(_ context.Context, deployment model.InstanceDeployment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, deployment)
	return nil
}

func (r *MemoryDeploymentRepository) ListByInstance(_ context.Context, instanceID string, limit int) ([]model.InstanceDeployment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.InstanceDeployment, 0)
	for _, item := range r.items {
		if item.InstanceID == instanceID {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

type MongoDeploymentRepository struct{ db *mongo.Database }

func NewMongoDeploymentRepository(db *mongo.Database) *MongoDeploymentRepository {
	return &MongoDeploymentRepository{db: db}
}

func (r *MongoDeploymentRepository) collection() *mongo.Collection {
	return r.db.Collection("instance_deployments")
}

func (r *MongoDeploymentRepository) Create(ctx context.Context, deployment model.InstanceDeployment) error {
	_, err := r.collection().InsertOne(ctx, deployment)
	return err
}

func (r *MongoDeploymentRepository) ListByInstance(ctx context.Context, instanceID string, limit int) ([]model.InstanceDeployment, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	cursor, err := r.collection().Find(ctx, bson.M{"instance_id": instanceID}, opts)
	if err != nil {
		return nil, err
	}
	var result []model.InstanceDeployment
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (a *InstanceService) recordDeployment(instanceID, operation, image, previousImage, operator string, operationErr error) {
	if a.deployments == nil || operation == "restart" {
		return
	}
	deployment := model.InstanceDeployment{
		ID:            uuid.NewString(),
		InstanceID:    instanceID,
		Operation:     operation,
		Image:         image,
		PreviousImage: previousImage,
		Status:        "succeeded",
		Operator:      operator,
		CreatedAt:     time.Now().UTC(),
	}
	if operationErr != nil {
		deployment.Status = "failed"
		deployment.Message = operationErr.Error()
	}
	_ = a.deployments.Create(bgCtx(), deployment)
}

func (a *InstanceService) recordDeploymentBaseline(instanceID, image string) {
	if a.deployments == nil || image == "" {
		return
	}
	existing, err := a.deployments.ListByInstance(bgCtx(), instanceID, 0)
	if err != nil {
		return
	}
	for _, item := range existing {
		if item.Status == "succeeded" && item.Image == image {
			return
		}
	}
	_ = a.deployments.Create(bgCtx(), model.InstanceDeployment{
		ID:         uuid.NewString(),
		InstanceID: instanceID,
		Operation:  "deploy",
		Image:      image,
		Status:     "succeeded",
		Message:    "升级前正在运行的版本（自动记录）",
		CreatedAt:  time.Now().UTC(),
	})
}

func (a *InstanceService) listDeployments(instanceID string, limit int) ([]model.InstanceDeployment, error) {
	if _, err := a.repo.Get(bgCtx(), instanceID); err != nil {
		return nil, err
	}
	deployments, err := a.deployments.ListByInstance(bgCtx(), instanceID, limit)
	if err != nil {
		return nil, err
	}
	if deployments == nil {
		deployments = []model.InstanceDeployment{}
	}
	return deployments, nil
}

func (a *InstanceService) rollbackDeployment(instanceID, deploymentID, operator string) (model.ServiceInstance, error) {
	item, err := a.repo.Get(bgCtx(), instanceID)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	deployments, err := a.deployments.ListByInstance(bgCtx(), instanceID, 0)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	var target *model.InstanceDeployment
	for i := range deployments {
		if deployments[i].ID == deploymentID {
			target = &deployments[i]
			break
		}
	}
	if target == nil || target.Status != "succeeded" || target.Image == "" {
		return model.ServiceInstance{}, model.ErrDeploymentUnavailable
	}
	if item.HostID == "" {
		return model.ServiceInstance{}, ErrHostUnresolved
	}
	return a.updateImageWithMetadata(instanceID, target.Image, operator, "rollback")
}
