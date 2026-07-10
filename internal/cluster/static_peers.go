package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/config"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/rs/zerolog/log"
)

type StaticPeerRegistry struct {
	mu     sync.RWMutex
	peers  map[string]*StaticPeer
	leader *LeaderNode
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type StaticPeer struct {
	ID       string
	Address  string
	Port     int
	LastSeen time.Time
	Healthy  bool
}

func NewStaticPeerRegistry(cfg []config.StaticPeerConfig, leader *LeaderNode) *StaticPeerRegistry {
	r := &StaticPeerRegistry{
		peers:  make(map[string]*StaticPeer),
		leader: leader,
	}

	for _, c := range cfg {
		r.peers[c.ID] = &StaticPeer{
			ID:      c.ID,
			Address: c.Address,
			Port:    c.Port,
			Healthy: false,
		}
	}

	return r
}

func (r *StaticPeerRegistry) Start(ctx context.Context) error {
	ctx, r.cancel = context.WithCancel(ctx)

	r.wg.Add(1)
	go r.run(ctx)

	log.Info().Int("count", len(r.peers)).Msg("Static peer registry started")

	// Initial connection attempt
	r.connectAll(ctx)

	return nil
}

func (r *StaticPeerRegistry) Stop() error {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	return nil
}

func (r *StaticPeerRegistry) run(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkHealth(ctx)
		}
	}
}

func (r *StaticPeerRegistry) connectAll(ctx context.Context) {
	r.mu.RLock()
	peers := make([]*StaticPeer, 0, len(r.peers))
	for _, p := range r.peers {
		peers = append(peers, p)
	}
	r.mu.RUnlock()

	for _, p := range peers {
		r.tryConnect(ctx, p)
	}
}

func (r *StaticPeerRegistry) tryConnect(ctx context.Context, p *StaticPeer) {
	url := fmt.Sprintf("http://%s:%d", p.Address, p.Port)
	log.Info().Str("peer_id", p.ID).Str("url", url).Msg("Attempting to connect to static peer")

	node := &protocol.NodeInfo{
		ID:      p.ID,
		Address: p.Address,
		Port:    p.Port,
		Role:    "peer",
	}

	r.leader.RegisterNode(node)
	log.Info().Str("peer_id", p.ID).Msg("Static peer registered successfully")
	p.Healthy = true
	p.LastSeen = time.Now()
}

func (r *StaticPeerRegistry) checkHealth(ctx context.Context) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.peers {
		if !p.Healthy {
			// Retry unhealthy peers
			r.tryConnect(ctx, p)
		}
	}
}

func (r *StaticPeerRegistry) GetPeer(id string) (*StaticPeer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, exists := r.peers[id]
	return p, exists
}

func (r *StaticPeerRegistry) ListPeers() []*StaticPeer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peers := make([]*StaticPeer, 0, len(r.peers))
	for _, p := range r.peers {
		peers = append(peers, p)
	}
	return peers
}

func (r *StaticPeerRegistry) CheckHealth(ctx context.Context) map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string]bool)
	for id, p := range r.peers {
		results[id] = p.Healthy
	}
	return results
}
