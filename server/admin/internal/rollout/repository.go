package rollout

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogu-x/ops/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	errRolloutLocked       = errors.New("实例已被其他操作占用")
	errRolloutStateChanged = errors.New("滚动发布任务状态已变更或仍在执行")
)

type Repository interface {
	ListTemplates(context.Context) ([]model.RolloutTemplate, error)
	CreateTemplate(context.Context, model.RolloutTemplate) error
	EnsureIndexes(context.Context) error
	Create(context.Context, model.RolloutJob, []model.RolloutItem, model.RolloutAuditEvent) error
	Get(context.Context, string) (model.RolloutJob, error)
	List(context.Context, []string, int) ([]model.RolloutJob, error)
	Items(context.Context, string) ([]model.RolloutItem, error)
	UpdateJob(context.Context, model.RolloutJob) error
	SetJobStatus(context.Context, string, string, string, ...string) error
	SetJobGroups(context.Context, string, []model.RolloutGroupPlan) error
	SetJobProgress(context.Context, string, int, int, int, int) error
	UpdateItem(context.Context, model.RolloutItem) error
	AppendEvent(context.Context, model.RolloutAuditEvent) error
	Events(context.Context, string, int) ([]model.RolloutAuditEvent, error)
	ClaimNext(context.Context, string, time.Time, time.Duration) (model.RolloutJob, bool, error)
	LeaseInactiveJob(context.Context, string, string, time.Time, ...string) error
	RenewLease(context.Context, string, string, time.Time) error
	ReleaseLease(context.Context, string, string, time.Time) error
	AcquireInstanceLocks(context.Context, string, []string) error
	ReleaseInstanceLocks(context.Context, string) error
	IsInstanceLocked(context.Context, string) (bool, error)
	ClearLegacyOperationLocks(context.Context) error
}

type MemoryRepository struct {
	templates []model.RolloutTemplate
	mu        sync.Mutex
	jobs      map[string]model.RolloutJob
	items     map[string][]model.RolloutItem
	events    map[string][]model.RolloutAuditEvent
	locks     map[string]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		jobs:   make(map[string]model.RolloutJob),
		items:  make(map[string][]model.RolloutItem),
		events: make(map[string][]model.RolloutAuditEvent),
		locks:  make(map[string]string),
	}
}

func (r *MemoryRepository) EnsureIndexes(context.Context) error { return nil }

func (r *MemoryRepository) Create(_ context.Context, job model.RolloutJob, items []model.RolloutItem, event model.RolloutAuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.jobs[job.ID]; exists {
		return errors.New("rollout already exists")
	}
	r.jobs[job.ID] = job
	r.items[job.ID] = append([]model.RolloutItem(nil), items...)
	r.events[job.ID] = []model.RolloutAuditEvent{event}
	return nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (model.RolloutJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return model.RolloutJob{}, model.ErrNotFound
	}
	return job, nil
}

func (r *MemoryRepository) List(_ context.Context, projectIDs []string, limit int) ([]model.RolloutJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	allowed := make(map[string]struct{}, len(projectIDs))
	for _, id := range projectIDs {
		allowed[id] = struct{}{}
	}
	result := make([]model.RolloutJob, 0)
	for _, job := range r.jobs {
		if len(allowed) > 0 {
			if _, ok := allowed[job.ProjectID]; !ok {
				continue
			}
		}
		result = append(result, job)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) Items(_ context.Context, id string) ([]model.RolloutItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[id]; !ok {
		return nil, model.ErrNotFound
	}
	return append([]model.RolloutItem(nil), r.items[id]...), nil
}

func (r *MemoryRepository) UpdateJob(_ context.Context, job model.RolloutJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[job.ID]; !ok {
		return model.ErrNotFound
	}
	r.jobs[job.ID] = job
	return nil
}

func (r *MemoryRepository) SetJobStatus(_ context.Context, id, status, reason string, from ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return model.ErrNotFound
	}
	if len(from) > 0 && !slices.Contains(from, job.Status) {
		return errRolloutStateChanged
	}
	job.Status, job.PauseReason, job.UpdatedAt = status, reason, time.Now().UTC()
	r.jobs[id] = job
	return nil
}

func (r *MemoryRepository) SetJobProgress(_ context.Context, id string, total, completed, succeeded, failed int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return model.ErrNotFound
	}
	job.Total, job.Completed, job.Succeeded, job.Failed = total, completed, succeeded, failed
	job.UpdatedAt = time.Now().UTC()
	r.jobs[id] = job
	return nil
}

func (r *MemoryRepository) SetJobGroups(_ context.Context, id string, groups []model.RolloutGroupPlan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return model.ErrNotFound
	}
	job.Groups = groups
	r.jobs[id] = job
	return nil
}

func (r *MemoryRepository) UpdateItem(_ context.Context, item model.RolloutItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items, ok := r.items[item.JobID]
	if !ok {
		return model.ErrNotFound
	}
	for i := range items {
		if items[i].ID == item.ID {
			items[i] = item
			r.items[item.JobID] = items
			return nil
		}
	}
	return model.ErrNotFound
}

func (r *MemoryRepository) AppendEvent(_ context.Context, event model.RolloutAuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[event.JobID] = append(r.events[event.JobID], event)
	return nil
}

func (r *MemoryRepository) Events(_ context.Context, id string, limit int) ([]model.RolloutAuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := append([]model.RolloutAuditEvent(nil), r.events[id]...)
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

func (r *MemoryRepository) ClaimNext(_ context.Context, worker string, now time.Time, lease time.Duration) (model.RolloutJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var candidate *model.RolloutJob
	for _, job := range r.jobs {
		if (job.Status != "queued" && job.Status != "running") || job.LeaseUntil.After(now) {
			continue
		}
		if candidate == nil || job.CreatedAt.Before(candidate.CreatedAt) {
			copy := job
			candidate = &copy
		}
	}
	if candidate == nil {
		return model.RolloutJob{}, false, nil
	}
	candidate.Status = "running"
	candidate.LeaseOwner = worker
	candidate.LeaseUntil = now.Add(lease)
	candidate.UpdatedAt = now
	r.jobs[candidate.ID] = *candidate
	return *candidate, true, nil
}

func (r *MemoryRepository) LeaseInactiveJob(_ context.Context, id, owner string, until time.Time, statuses ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return model.ErrNotFound
	}
	if !slices.Contains(statuses, job.Status) || job.LeaseUntil.After(time.Now().UTC()) {
		return errRolloutStateChanged
	}
	job.LeaseOwner, job.LeaseUntil = owner, until
	r.jobs[id] = job
	return nil
}

func (r *MemoryRepository) RenewLease(_ context.Context, id, worker string, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok || job.LeaseOwner != worker || !job.LeaseUntil.After(time.Now().UTC()) {
		return model.ErrNotFound
	}
	job.LeaseUntil = until
	job.UpdatedAt = time.Now().UTC()
	r.jobs[id] = job
	return nil
}

func (r *MemoryRepository) ReleaseLease(_ context.Context, id, worker string, nextClaim time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok || job.LeaseOwner != worker {
		return model.ErrNotFound
	}
	job.LeaseOwner = ""
	job.LeaseUntil = nextClaim
	r.jobs[id] = job
	return nil
}

func (r *MemoryRepository) AcquireInstanceLocks(_ context.Context, owner string, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for _, id := range ids {
		if current, ok := r.locks[id]; ok && current != owner {
			jobID, leaseOwner, rollout := parseRolloutLockOwner(current)
			if !rollout || rolloutLockActive(r.jobs[jobID], leaseOwner, owner, now) {
				return errRolloutLocked
			}
		}
	}
	for _, id := range ids {
		r.locks[id] = owner
	}
	return nil
}

func (r *MemoryRepository) ReleaseInstanceLocks(_ context.Context, owner string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, current := range r.locks {
		if current == owner {
			delete(r.locks, id)
		}
	}
	return nil
}

func (r *MemoryRepository) IsInstanceLocked(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.locks[id]
	return ok, nil
}

func (r *MemoryRepository) ClearLegacyOperationLocks(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, owner := range r.locks {
		if len(owner) < len("rollout:") || owner[:len("rollout:")] != "rollout:" {
			delete(r.locks, id)
		}
	}
	return nil
}

type MongoRepository struct{ db *mongo.Database }

func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{db: db}
}

func (r *MongoRepository) templates() *mongo.Collection {
	return r.db.Collection("rollout_templates")
}

func (r *MongoRepository) jobs() *mongo.Collection   { return r.db.Collection("rollout_jobs") }
func (r *MongoRepository) items() *mongo.Collection  { return r.db.Collection("rollout_items") }
func (r *MongoRepository) events() *mongo.Collection { return r.db.Collection("rollout_events") }
func (r *MongoRepository) locks() *mongo.Collection  { return r.db.Collection("rollout_locks") }

func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	indexes := []struct {
		collection *mongo.Collection
		models     []mongo.IndexModel
	}{
		{r.jobs(), []mongo.IndexModel{{Keys: bson.D{{Key: "status", Value: 1}, {Key: "lease_until", Value: 1}, {Key: "created_at", Value: 1}}, Options: options.Index().SetName("rollout_worker_claim")}, {Keys: bson.D{{Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("rollout_project_history")}}},
		{r.items(), []mongo.IndexModel{{Keys: bson.D{{Key: "job_id", Value: 1}, {Key: "instance_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("rollout_item_unique")}, {Keys: bson.D{{Key: "job_id", Value: 1}, {Key: "status", Value: 1}, {Key: "group_key", Value: 1}}, Options: options.Index().SetName("rollout_item_schedule")}}},
		{r.events(), []mongo.IndexModel{{Keys: bson.D{{Key: "job_id", Value: 1}, {Key: "created_at", Value: 1}}, Options: options.Index().SetName("rollout_event_history")}}},
	}
	for _, entry := range indexes {
		if _, err := entry.collection.Indexes().CreateMany(ctx, entry.models); err != nil {
			return err
		}
	}
	return nil
}

func (r *MongoRepository) Create(ctx context.Context, job model.RolloutJob, items []model.RolloutItem, event model.RolloutAuditEvent) error {
	// Publish the queued job only after its items and initial event are available to workers.
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = r.items().DeleteMany(cleanupCtx, bson.M{"job_id": job.ID})
		_, _ = r.events().DeleteOne(cleanupCtx, bson.M{"_id": event.ID})
	}
	if len(items) > 0 {
		docs := make([]interface{}, len(items))
		for i := range items {
			docs[i] = items[i]
		}
		if _, err := r.items().InsertMany(ctx, docs); err != nil {
			cleanup()
			return err
		}
	}
	if _, err := r.events().InsertOne(ctx, event); err != nil {
		cleanup()
		return err
	}
	if _, err := r.jobs().InsertOne(ctx, job); err != nil {
		verifyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stored, getErr := r.Get(verifyCtx, job.ID)
		if getErr == nil && stored.ID == job.ID {
			return nil
		}
		if errors.Is(getErr, model.ErrNotFound) {
			cleanup()
		}
		return err
	}
	return nil
}

func (r *MongoRepository) Get(ctx context.Context, id string) (model.RolloutJob, error) {
	var job model.RolloutJob
	err := r.jobs().FindOne(ctx, bson.M{"_id": id}).Decode(&job)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.RolloutJob{}, model.ErrNotFound
	}
	return job, err
}

func (r *MongoRepository) List(ctx context.Context, projectIDs []string, limit int) ([]model.RolloutJob, error) {
	filter := bson.M{}
	if len(projectIDs) > 0 {
		filter["project_id"] = bson.M{"$in": projectIDs}
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	cursor, err := r.jobs().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var result []model.RolloutJob
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *MongoRepository) Items(ctx context.Context, id string) ([]model.RolloutItem, error) {
	cursor, err := r.items().Find(ctx, bson.M{"job_id": id}, options.Find().SetSort(bson.D{{Key: "group_key", Value: 1}, {Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var items []model.RolloutItem
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MongoRepository) UpdateJob(ctx context.Context, job model.RolloutJob) error {
	_, err := r.jobs().ReplaceOne(ctx, bson.M{"_id": job.ID}, job)
	return err
}

func (r *MongoRepository) SetJobStatus(ctx context.Context, id, status, reason string, from ...string) error {
	filter := bson.M{"_id": id}
	if len(from) > 0 {
		filter["status"] = bson.M{"$in": from}
	}
	result, err := r.jobs().UpdateOne(ctx, filter, bson.M{"$set": bson.M{"status": status, "pause_reason": reason, "updated_at": time.Now().UTC()}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errRolloutStateChanged
	}
	return nil
}

func (r *MongoRepository) SetJobProgress(ctx context.Context, id string, total, completed, succeeded, failed int) error {
	_, err := r.jobs().UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"total": total, "completed": completed, "succeeded": succeeded, "failed": failed, "updated_at": time.Now().UTC()}})
	return err
}

func (r *MongoRepository) SetJobGroups(ctx context.Context, id string, groups []model.RolloutGroupPlan) error {
	_, err := r.jobs().UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"groups": groups}})
	return err
}

func (r *MongoRepository) UpdateItem(ctx context.Context, item model.RolloutItem) error {
	_, err := r.items().ReplaceOne(ctx, bson.M{"_id": item.ID, "job_id": item.JobID}, item)
	return err
}

func (r *MongoRepository) AppendEvent(ctx context.Context, event model.RolloutAuditEvent) error {
	_, err := r.events().InsertOne(ctx, event)
	return err
}

func (r *MongoRepository) Events(ctx context.Context, id string, limit int) ([]model.RolloutAuditEvent, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	cursor, err := r.events().Find(ctx, bson.M{"job_id": id}, opts)
	if err != nil {
		return nil, err
	}
	var events []model.RolloutAuditEvent
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
		events[left], events[right] = events[right], events[left]
	}
	return events, nil
}

func (r *MongoRepository) ClaimNext(ctx context.Context, worker string, now time.Time, lease time.Duration) (model.RolloutJob, bool, error) {
	filter := unleasedRolloutJobs(now, "queued", "running")
	update := bson.M{"$set": bson.M{"status": "running", "lease_owner": worker, "lease_until": now.Add(lease), "updated_at": now}}
	var job model.RolloutJob
	err := r.jobs().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetReturnDocument(options.After)).Decode(&job)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.RolloutJob{}, false, nil
	}
	return job, err == nil, err
}

func unleasedRolloutJobs(now time.Time, statuses ...string) bson.M {
	return bson.M{"status": bson.M{"$in": statuses}, "$or": []bson.M{{"lease_until": bson.M{"$exists": false}}, {"lease_until": bson.M{"$lte": now}}}}
}

func (r *MongoRepository) LeaseInactiveJob(ctx context.Context, id, owner string, until time.Time, statuses ...string) error {
	filter := unleasedRolloutJobs(time.Now().UTC(), statuses...)
	filter["_id"] = id
	result, err := r.jobs().UpdateOne(ctx, filter, bson.M{"$set": bson.M{"lease_owner": owner, "lease_until": until}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errRolloutStateChanged
	}
	return nil
}

func (r *MongoRepository) RenewLease(ctx context.Context, id, worker string, until time.Time) error {
	now := time.Now().UTC()
	result, err := r.jobs().UpdateOne(ctx, bson.M{"_id": id, "lease_owner": worker, "lease_until": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"lease_until": until, "updated_at": now}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *MongoRepository) ReleaseLease(ctx context.Context, id, worker string, nextClaim time.Time) error {
	_, err := r.jobs().UpdateOne(ctx, bson.M{"_id": id, "lease_owner": worker}, bson.M{"$set": bson.M{"lease_until": nextClaim}, "$unset": bson.M{"lease_owner": ""}})
	return err
}

func (r *MongoRepository) AcquireInstanceLocks(ctx context.Context, owner string, ids []string) error {
	if len(ids) == 0 {
		return errors.New("没有要锁定的实例")
	}
	err := r.insertInstanceLocks(ctx, owner, ids)
	if !errors.Is(err, errRolloutLocked) {
		return err
	}
	cleared, err := r.clearInactiveRolloutLocks(ctx, owner, ids)
	if err != nil {
		return err
	}
	if !cleared {
		return errRolloutLocked
	}
	return r.insertInstanceLocks(ctx, owner, ids)
}

func (r *MongoRepository) insertInstanceLocks(ctx context.Context, owner string, ids []string) error {
	now := time.Now().UTC()
	docs := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		docs = append(docs, bson.M{"_id": id, "owner_id": owner, "created_at": now})
	}
	_, err := r.locks().InsertMany(ctx, docs, options.InsertMany().SetOrdered(true))
	if err == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if cleanupErr := r.ReleaseInstanceLocks(cleanupCtx, owner); cleanupErr != nil {
		return errors.Join(err, cleanupErr)
	}
	if mongo.IsDuplicateKeyError(err) || strings.Contains(err.Error(), "E11000") || strings.Contains(err.Error(), "duplicate key") {
		return errRolloutLocked
	}
	return err
}

func (r *MongoRepository) ReleaseInstanceLocks(ctx context.Context, owner string) error {
	_, err := r.locks().DeleteMany(ctx, bson.M{"owner_id": owner})
	return err
}

func (r *MongoRepository) clearInactiveRolloutLocks(ctx context.Context, owner string, ids []string) (bool, error) {
	cursor, err := r.locks().Find(ctx, bson.M{"_id": bson.M{"$in": ids}, "owner_id": bson.M{"$regex": "^rollout:"}})
	if err != nil {
		return false, err
	}
	var locks []struct {
		OwnerID string `bson:"owner_id"`
	}
	if err := cursor.All(ctx, &locks); err != nil {
		return false, err
	}
	seen := make(map[string]struct{})
	var inactive []string
	for _, lock := range locks {
		if _, ok := seen[lock.OwnerID]; ok {
			continue
		}
		seen[lock.OwnerID] = struct{}{}
		jobID, leaseOwner, _ := parseRolloutLockOwner(lock.OwnerID)
		now := time.Now().UTC()
		job, err := r.Get(ctx, jobID)
		if err != nil && !errors.Is(err, model.ErrNotFound) {
			return false, err
		}
		if !rolloutLockActive(job, leaseOwner, owner, now) {
			inactive = append(inactive, lock.OwnerID)
		}
	}
	if len(inactive) == 0 {
		return false, nil
	}
	_, err = r.locks().DeleteMany(ctx, bson.M{"owner_id": bson.M{"$in": inactive}})
	return err == nil, err
}

func parseRolloutLockOwner(owner string) (jobID, leaseOwner string, rollout bool) {
	rest, rollout := strings.CutPrefix(owner, "rollout:")
	jobID, leaseOwner, _ = strings.Cut(rest, ":")
	return jobID, leaseOwner, rollout
}

func rolloutLockActive(job model.RolloutJob, leaseOwner, newOwner string, now time.Time) bool {
	if leaseOwner != "" {
		return job.LeaseOwner == leaseOwner && job.LeaseUntil.After(now)
	}
	// Legacy locks identified only the job. A new claim of that same job supersedes them.
	return job.LeaseOwner != "" && job.LeaseUntil.After(now) && rolloutLockOwner(job.ID, job.LeaseOwner) != newOwner
}

func (r *MongoRepository) IsInstanceLocked(ctx context.Context, id string) (bool, error) {
	err := r.locks().FindOne(ctx, bson.M{"_id": id}).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

func (r *MongoRepository) ClearLegacyOperationLocks(ctx context.Context) error {
	_, err := r.locks().DeleteMany(ctx, bson.M{"owner_id": bson.M{"$not": bson.M{"$regex": "^rollout:"}}})
	return err
}

func (r *MemoryRepository) ListTemplates(_ context.Context) ([]model.RolloutTemplate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]model.RolloutTemplate, 0, len(r.templates))
	for _, item := range r.templates {
		if item.Kind == "release" {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *MemoryRepository) CreateTemplate(_ context.Context, item model.RolloutTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates = append(r.templates, item)
	return nil
}

func (r *MongoRepository) ListTemplates(ctx context.Context) ([]model.RolloutTemplate, error) {
	cursor, err := r.templates().Find(ctx, bson.M{"kind": "release"})
	if err != nil {
		return nil, err
	}
	items := make([]model.RolloutTemplate, 0)
	err = cursor.All(ctx, &items)
	return items, err
}

func (r *MongoRepository) CreateTemplate(ctx context.Context, item model.RolloutTemplate) error {
	_, err := r.templates().InsertOne(ctx, item)
	return err
}
