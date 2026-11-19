package rollout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gogu-x/ops/model"
)

func TestCreateRolloutForAnInstanceInAnExistingJob(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	manager := NewManager(repo, nil)
	create := func() model.RolloutJob {
		t.Helper()
		job, err := manager.Create(model.RolloutJob{}, []model.RolloutItem{{InstanceID: "instance"}})
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != "queued" {
			t.Fatalf("created job status = %q, want queued", job.Status)
		}
		return job
	}

	first := create()
	now := time.Now().UTC()
	claimed, ok, err := repo.ClaimNext(ctx, "first-claim", now, rolloutLeaseDuration)
	if err != nil || !ok || claimed.ID != first.ID {
		t.Fatalf("claim first job = (%s, %t, %v)", claimed.ID, ok, err)
	}
	ids := []string{"instance"}
	owner := rolloutLockOwner(first.ID, claimed.LeaseOwner)
	if err := repo.AcquireInstanceLocks(ctx, owner, ids); err != nil {
		t.Fatal(err)
	}
	second := create()

	if err := manager.Pause(first.ID, "operator", "pause"); err != nil {
		t.Fatal(err)
	}
	third := create()
	if third.ID == second.ID {
		t.Fatal("created jobs have the same ID")
	}
	if err := repo.AcquireInstanceLocks(ctx, "request:manual", ids); !errors.Is(err, errRolloutLocked) {
		t.Fatalf("instance lock while the paused job is draining = %v, want busy", err)
	}
	if err := repo.ReleaseInstanceLocks(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReleaseLease(ctx, first.ID, claimed.LeaseOwner, time.Time{}); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err = repo.ClaimNext(ctx, "next-claim", time.Now().UTC(), rolloutLeaseDuration)
	if err != nil || !ok || claimed.ID != second.ID {
		t.Fatalf("claim after pause = (%s, %t, %v), want %s", claimed.ID, ok, err, second.ID)
	}
	if err := repo.AcquireInstanceLocks(ctx, rolloutLockOwner(claimed.ID, claimed.LeaseOwner), ids); err != nil {
		t.Fatalf("claim instance after pause: %v", err)
	}
}

func TestReclaimInactiveRolloutInstanceLocks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		owner  string
	}{
		{"legacy paused job", "paused", "rollout:old"},
		{"legacy failed job", "failed", "rollout:old"},
		{"expired paused claim", "paused", rolloutLockOwner("old", "expired")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := NewMemoryRepository()
			old := model.RolloutJob{
				ID:         "old",
				Status:     tc.status,
				LeaseOwner: "expired",
				LeaseUntil: time.Now().Add(-time.Minute),
			}
			if err := repo.Create(ctx, old, nil, model.RolloutAuditEvent{}); err != nil {
				t.Fatal(err)
			}
			ids := []string{"instance"}
			if err := repo.AcquireInstanceLocks(ctx, tc.owner, ids); err != nil {
				t.Fatal(err)
			}
			manager := NewManager(repo, nil)
			job, err := manager.Create(model.RolloutJob{}, []model.RolloutItem{{InstanceID: ids[0]}})
			if err != nil {
				t.Fatal(err)
			}
			claimed, ok, err := repo.ClaimNext(ctx, "next-claim", time.Now().UTC(), rolloutLeaseDuration)
			if err != nil || !ok || claimed.ID != job.ID {
				t.Fatalf("claim job after old %s job = (%s, %t, %v)", tc.status, claimed.ID, ok, err)
			}
			if err := repo.AcquireInstanceLocks(ctx, rolloutLockOwner(job.ID, claimed.LeaseOwner), ids); err != nil {
				t.Fatalf("reclaim previous instance lock: %v", err)
			}
		})
	}
}

func TestBusyRolloutWaitsInQueue(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	manager := NewManager(repo, nil)
	if err := repo.AcquireInstanceLocks(ctx, "request:manual", []string{"busy"}); err != nil {
		t.Fatal(err)
	}
	blocked, err := manager.Create(model.RolloutJob{}, []model.RolloutItem{{InstanceID: "busy"}})
	if err != nil {
		t.Fatal(err)
	}
	manager.claimAndRun(ctx)
	waiting, err := repo.Get(ctx, blocked.ID)
	if err != nil || waiting.Status != "queued" || waiting.LeaseOwner != "" {
		t.Fatalf("rollout waiting on an instance lock = (%+v, %v)", waiting, err)
	}
	next, err := manager.Create(model.RolloutJob{}, []model.RolloutItem{{InstanceID: "free"}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := repo.ClaimNext(ctx, "next-claim", waiting.LeaseUntil.Add(-time.Millisecond), rolloutLeaseDuration)
	if err != nil || !ok || claimed.ID != next.ID {
		t.Fatalf("claim while blocked job waits = (%s, %t, %v), want %s", claimed.ID, ok, err, next.ID)
	}
}
