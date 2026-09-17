package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/georgik/espbrew-go/internal/cluster"
	"github.com/georgik/espbrew-go/internal/config"
	httpserver "github.com/georgik/espbrew-go/internal/http"
	"github.com/georgik/espbrew-go/internal/persistence"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "espbrew",
	Short: "ESP32 cluster flashing tool",
}

// Build information. These are injected at build time via -ldflags, e.g.:
//
//	go build -ldflags "-s -w -X main.Version=v0.4.0 -X main.BuildTime=2026-09-16T07:59:00Z" \
//	    -o espbrew ./cmd/espbrew
//
// Official releases (see .github/workflows/release.yml and scripts/build-release.sh)
// pass the git tag as Version; local/dev builds default to "dev" and always set
// a BuildTime timestamp, so `espbrew --version` can tell an official release apart
// from a custom build.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// versionCmd reports the build version information.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s version %s\n", rootCmd.Name(), versionString())
	},
}

var cfg struct {
	role        string
	bindAddr    string
	httpPort    int
	leaderAddr  string
	nodeID      string
	logLevel    string
	cfgFile     string
	workers     int
	disablemDNS bool
	devMode     bool
}

func init() {
	// flashCmd and monitorCmd added by their own init() functions
	rootCmd.AddCommand(clusterCmd)
	rootCmd.AddCommand(versionCmd)
	// Registering a non-empty Version makes cobra add a global `--version`
	// flag and print the banner for `espbrew --version` (and `espbrew version`).
	rootCmd.Version = versionString()
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
	clusterCmd.Flags().StringVar(&cfg.leaderAddr, "leader", os.Getenv("ESPBREW_LEADER"), "Leader address (for peers)")
	clusterCmd.Flags().StringVar(&cfg.nodeID, "node-id", "", "Node ID (default: hostname)")
	clusterCmd.Flags().StringVar(&cfg.logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	clusterCmd.Flags().IntVar(&cfg.workers, "workers", 2, "Number of flash workers")
	clusterCmd.Flags().BoolVar(&cfg.disablemDNS, "no-mdns", false, "Disable mDNS discovery")
	clusterCmd.Flags().BoolVar(&cfg.devMode, "dev-mode", false, "Enable developer mode (unsafe for production)")
}

func main() {
	// Set up console logging for all commands
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})

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

	// Get home directory for database
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	espbrewDir := filepath.Join(homeDir, ".espbrew")
	dbPath := filepath.Join(espbrewDir, "espbrew.db")

	// Ensure espbrew directory exists
	if err := os.MkdirAll(espbrewDir, 0755); err != nil {
		return fmt.Errorf("create espbrew directory: %w", err)
	}

	// Open persistence store
	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		return fmt.Errorf("open persistence store: %w", err)
	}
	defer store.Close()

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

	// Determine node ID: use provided flag, hostname, or random
	nodeID := cfg.nodeID
	if nodeID == "" {
		if hostname, err := os.Hostname(); err == nil {
			nodeID = hostname
		} else {
			nodeID = "node-" + randomID(8)
		}
	}

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
			StaticPeers:       appCfg.StaticPeers,
			Devices:           appCfg.Devices,
			ConfigPath:        config.ResolveConfigPath(cfg.cfgFile),
		}, store)
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
		}, store)
		node = leader
		if err := node.Start(ctx); err != nil {
			return err
		}
	}

	srv := httpserver.NewServer(addr, node, store)

	// Enable developer mode if requested
	if cfg.devMode {
		srv.SetDevMode(true)
	}

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

// versionString returns the human-readable version banner shown by
// `espbrew --version`, `espbrew version`, and the help output. It combines the
// build version (the git tag for releases, "dev" for custom builds) with the
// build timestamp so different builds can be told apart.
func versionString() string {
	v := strings.TrimSpace(Version)
	if v == "" {
		v = "dev"
	}
	b := strings.TrimSpace(BuildTime)
	if b == "" {
		b = "unknown"
	}
	return fmt.Sprintf("%s (built %s)", v, b)
}

func randomID(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
