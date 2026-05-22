package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/config"
	httpserver "codeberg.org/georgik/espbrew-go/internal/http"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "espbrew",
	Short: "ESP32 cluster flashing tool",
}

var cfg struct {
	role        string
	bindAddr    string
	httpPort    int
	leaderAddr  string
	logLevel    string
	cfgFile     string
	workers     int
	disablemDNS bool
}

func init() {
	// flashCmd and monitorCmd added by their own init() functions
	rootCmd.AddCommand(clusterCmd)
}

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Start ESPBrew cluster node",
	RunE:  runServer,
}

func init() {
	clusterCmd.Flags().StringVarP(&cfg.cfgFile, "config", "c", "", "Config file path")
	clusterCmd.Flags().StringVarP(&cfg.role, "role", "r", "standalone", "Node role: leader, peer, standalone")
	clusterCmd.Flags().StringVar(&cfg.bindAddr, "bind", "0.0.0.0", "Bind address")
	clusterCmd.Flags().IntVarP(&cfg.httpPort, "port", "p", 8080, "HTTP port")
	clusterCmd.Flags().StringVar(&cfg.leaderAddr, "leader", "", "Leader address (for peers)")
	clusterCmd.Flags().StringVar(&cfg.logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	clusterCmd.Flags().IntVar(&cfg.workers, "workers", 2, "Number of flash workers")
	clusterCmd.Flags().BoolVar(&cfg.disablemDNS, "no-mdns", false, "Disable mDNS discovery")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	level, _ := zerolog.ParseLevel(cfg.logLevel)
	zerolog.SetGlobalLevel(level)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	appCfg, _ := config.Load(cfg.cfgFile)
	if appCfg == nil {
		appCfg = config.Default()
	}

	if cmd.Flags().Changed("role") {
		appCfg.Role = cfg.role
	}
	if cmd.Flags().Changed("bind") {
		appCfg.BindAddress = cfg.bindAddr
	}
	if cmd.Flags().Changed("port") {
		appCfg.HTTPPort = cfg.httpPort
	}
	if cmd.Flags().Changed("leader") {
		appCfg.LeaderAddress = cfg.leaderAddr
	}

	nodeID := "node-" + randomID(8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var node cluster.Node
	addr := fmt.Sprintf("%s:%d", appCfg.BindAddress, appCfg.HTTPPort)

	switch appCfg.Role {
	case "leader":
		leader := cluster.NewLeaderNode(nodeID, &cluster.LeaderConfig{
			HeartbeatInterval: appCfg.HeartbeatInterval,
			NodeTimeout:       appCfg.NodeTimeout,
			HTTPPort:          appCfg.HTTPPort,
			DisablemDNS:       cfg.disablemDNS,
		})
		node = leader
		if err := node.Start(ctx); err != nil {
			return err
		}

	case "peer":
		if appCfg.LeaderAddress == "" {
			return fmt.Errorf("peer requires --leader address")
		}
		peer := cluster.NewPeerNode(nodeID, appCfg.LeaderAddress, &cluster.PeerConfig{
			HeartbeatInterval: appCfg.HeartbeatInterval,
			HTTPPort:          appCfg.HTTPPort,
			DisablemDNS:       cfg.disablemDNS,
			DisableWatcher:    false,
		})
		node = peer
		if err := node.Start(ctx); err != nil {
			return err
		}

	default:
		// Standalone mode - leader with device discovery
		leader := cluster.NewLeaderNode(nodeID, &cluster.LeaderConfig{
			HeartbeatInterval: appCfg.HeartbeatInterval,
			NodeTimeout:       appCfg.NodeTimeout,
			HTTPPort:          appCfg.HTTPPort,
			DisablemDNS:       true,
		})
		node = leader
		if err := node.Start(ctx); err != nil {
			return err
		}
	}

	srv := httpserver.NewServer(addr, node)

	// Start executor with progress callback for leader nodes
	if l, ok := node.(*cluster.LeaderNode); ok {
		progressCB := srv.GetProgressCallback()
		l.StartJobExecutorWithProgress(cfg.workers, progressCB)
	}

	if err := srv.Start(ctx); err != nil {
		return err
	}

	log.Info().
		Str("role", appCfg.Role).
		Str("addr", addr).
		Str("node_id", nodeID).
		Int("workers", cfg.workers).
		Msg("ESPBrew cluster running")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info().Msg("Shutting down...")
	cancel()

	done := make(chan struct{})
	go func() {
		if l, ok := node.(*cluster.LeaderNode); ok {
			l.StopJobExecutor()
		}
		node.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		log.Warn().Msg("Shutdown timeout, forcing exit")
	}

	return nil
}

func randomID(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
