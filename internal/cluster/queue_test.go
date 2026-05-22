package cluster

import (
	"testing"
	"time"
)

func TestJobQueue_Cancel(t *testing.T) {
	q := NewJobQueue()

	job := q.Enqueue("test.bin", "/dev/ttyUSB0")

	if job.Status != JobPending {
		t.Errorf("expected pending, got %s", job.Status)
	}

	err := q.Cancel(job.ID)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	job.RLock()
	if job.Status != JobCancelled {
		t.Errorf("expected cancelled, got %s", job.Status)
	}
	if job.Error != "Cancelled by user" {
		t.Errorf("expected cancellation message, got %s", job.Error)
	}
	job.RUnlock()
}

func TestJobQueue_CancelCompletedJob(t *testing.T) {
	q := NewJobQueue()

	job := q.Enqueue("test.bin", "/dev/ttyUSB0")

	// Mark as completed
	q.Complete(job.ID, nil)

	err := q.Cancel(job.ID)
	if err == nil {
		t.Fatal("expected error cancelling completed job")
	}
}

func TestJobQueue_CancelNotFound(t *testing.T) {
	q := NewJobQueue()

	err := q.Cancel("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent job")
	}
}

func TestJobQueue_Timeout(t *testing.T) {
	q := NewJobQueue()

	job := q.Enqueue("test.bin", "/dev/ttyUSB0")

	err := q.Timeout(job.ID)
	if err != nil {
		t.Fatalf("timeout failed: %v", err)
	}

	job.RLock()
	if job.Status != JobTimedOut {
		t.Errorf("expected timeout, got %s", job.Status)
	}
	if job.Error != "Job timed out" {
		t.Errorf("expected timeout message, got %s", job.Error)
	}
	job.RUnlock()
}

func TestJobQueue_CleanupOld(t *testing.T) {
	q := NewJobQueue()

	// Create some jobs
	job1 := q.Enqueue("test1.bin", "/dev/ttyUSB0")
	job2 := q.Enqueue("test2.bin", "/dev/ttyUSB1")
	job3 := q.Enqueue("test3.bin", "/dev/ttyUSB2")

	// Complete job1 and job2, leave job3 pending
	q.Complete(job1.ID, nil)
	q.Complete(job2.ID, nil)

	// Manually set completion time to old
	oldTime := time.Now().Add(-48 * time.Hour)
	job1.mu.Lock()
	job1.CompletedAt = &oldTime
	job1.mu.Unlock()

	// Clean up jobs older than 24 hours
	removed := q.CleanupOld(24 * time.Hour)

	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	// Verify job1 is gone
	if q.Get(job1.ID) != nil {
		t.Error("job1 should be removed")
	}

	// Verify job2 and job3 still exist
	if q.Get(job2.ID) == nil {
		t.Error("job2 should still exist")
	}
	if q.Get(job3.ID) == nil {
		t.Error("job3 should still exist")
	}
}

func TestJobQueue_CleanupOldPending(t *testing.T) {
	q := NewJobQueue()

	job := q.Enqueue("test.bin", "/dev/ttyUSB0")

	// Pending jobs should not be cleaned up even if old
	removed := q.CleanupOld(0)
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}

	if q.Get(job.ID) == nil {
		t.Error("pending job should not be removed")
	}
}

func TestJobStatuses(t *testing.T) {
	statuses := []JobStatus{
		JobPending,
		JobAssigned,
		JobRunning,
		JobComplete,
		JobFailed,
		JobCancelled,
		JobTimedOut,
	}

	for _, status := range statuses {
		if status == "" {
			t.Errorf("empty status string")
		}
	}
}

func TestJobTimeoutConstants(t *testing.T) {
	if DefaultJobTimeout < 5*time.Minute {
		t.Errorf("DefaultJobTimeout too short: %v", DefaultJobTimeout)
	}

	if DefaultJobTTL < 1*time.Hour {
		t.Errorf("DefaultJobTTL too short: %v", DefaultJobTTL)
	}
}
