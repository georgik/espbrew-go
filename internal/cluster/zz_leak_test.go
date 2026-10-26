package cluster

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/georgik/espbrew-go/internal/persistence"
)

func TestZZLeak(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/db.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for i := 0; i < 40; i++ {
		l := NewLeaderNode("t", &LeaderConfig{
			HeartbeatInterval: 10 * time.Second, NodeTimeout: 30 * time.Second,
			HTTPPort: 8080, DisablemDNS: true, DisableWatcher: true,
			DisableMaintenance: true, DisableVirtual: true,
		}, store)
		_ = l.Start(context.Background())
		_ = l.Stop()
		if i%10 == 0 {
			g := runtime.NumGoroutine()
			t.Logf("after %d cycles: %d goroutines", i, g)
		}
	}
}
