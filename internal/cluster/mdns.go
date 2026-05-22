package cluster

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/rs/zerolog/log"
)

const (
	serviceName   = "_espbrew._tcp"
	serviceDomain = "local."
)

type mdnsInfo struct {
	nodeID  string
	role    string
	address string
	port    int
}

type mDNSService struct {
	nodeID   string
	role     string
	httpPort int
	entries  chan *mdns.ServiceEntry
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.RWMutex
	peers    map[string]*PeerInfo
}

type PeerInfo struct {
	NodeID   string
	Role     string
	Address  string
	Port     int
	LastSeen time.Time
}

func NewmDNSService(nodeID, role string, httpPort int) *mDNSService {
	ctx, cancel := context.WithCancel(context.Background())
	return &mDNSService{
		nodeID:   nodeID,
		role:     role,
		httpPort: httpPort,
		entries:  make(chan *mdns.ServiceEntry, 10),
		ctx:      ctx,
		cancel:   cancel,
		peers:    make(map[string]*PeerInfo),
	}
}

func (m *mDNSService) Start() error {
	log.Info().Str("node_id", m.nodeID).Str("role", m.role).
		Msg("Starting mDNS service")

	// Start announcer
	m.wg.Add(1)
	go m.announce()

	// Start discovery
	m.wg.Add(1)
	go m.discover()

	// Start entry processor
	m.wg.Add(1)
	go m.processEntries()

	// Start peer cleanup
	m.wg.Add(1)
	go m.cleanupLoop()

	return nil
}

func (m *mDNSService) Stop() error {
	m.cancel()
	m.wg.Wait()
	close(m.entries)
	return nil
}

func (m *mDNSService) Peers() []*PeerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peers := make([]*PeerInfo, 0, len(m.peers))
	for _, p := range m.peers {
		peers = append(peers, p)
	}
	return peers
}

func (m *mDNSService) announce() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	info := []string{
		fmt.Sprintf("node_id=%s", m.nodeID),
		fmt.Sprintf("role=%s", m.role),
	}

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		// Get local IP
		ip, err := m.getLocalIP()
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get local IP for mDNS announcement")
			time.Sleep(5 * time.Second)
			continue
		}

		service, err := mdns.NewMDNSService(
			m.nodeID,
			serviceName,
			serviceDomain,
			"",
			m.httpPort,
			[]net.IP{ip},
			info,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create mDNS service")
			return
		}

		server, err := mdns.NewServer(&mdns.Config{Zone: service})
		if err != nil {
			log.Error().Err(err).Msg("Failed to start mDNS server")
			return
		}

		log.Info().Str("ip", ip.String()).Int("port", m.httpPort).
			Msg("mDNS announcement sent")

		// Announce once, then wait for ticker
		<-time.After(1 * time.Second)
		server.Shutdown()

		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *mDNSService) discover() {
	defer m.wg.Done()

	params := &mdns.QueryParam{
		Service:             serviceName,
		Domain:              serviceDomain,
		Timeout:             time.Minute * 15,
		Entries:             m.entries,
		WantUnicastResponse: false,
	}

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		if err := mdns.Query(params); err != nil {
			log.Debug().Err(err).Msg("mDNS query failed")
			time.Sleep(5 * time.Second)
			continue
		}
	}
}

func (m *mDNSService) processEntries() {
	for entry := range m.entries {
		if len(entry.InfoFields) < 2 {
			continue
		}

		nodeID := ""
		role := ""
		for _, field := range entry.InfoFields {
			if len(field) > 8 && field[:8] == "node_id=" {
				nodeID = field[8:]
			}
			if len(field) > 5 && field[:5] == "role=" {
				role = field[5:]
			}
		}

		if nodeID == "" || nodeID == m.nodeID {
			continue // Skip self
		}

		m.mu.Lock()
		m.peers[nodeID] = &PeerInfo{
			NodeID:   nodeID,
			Role:     role,
			Address:  entry.AddrV4.String(),
			Port:     entry.Port,
			LastSeen: time.Now(),
		}
		m.mu.Unlock()

		log.Info().Str("peer", nodeID).Str("role", role).
			Str("address", entry.AddrV4.String()).Msg("Peer discovered")
	}
}

func (m *mDNSService) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupStale()
		}
	}
}

func (m *mDNSService) cleanupStale() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	timeout := 2 * time.Minute

	for id, peer := range m.peers {
		if now.Sub(peer.LastSeen) > timeout {
			delete(m.peers, id)
			log.Info().Str("peer", id).Msg("Peer timed out")
		}
	}
}

func (m *mDNSService) getLocalIP() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP, nil
			}
		}
	}

	return nil, fmt.Errorf("no local IP found")
}
