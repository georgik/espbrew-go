package main

import (
	"context"
	"errors"
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

	// On error we print the message ourselves (see printTopLevelError):
	// SilenceErrors stops cobra double-printing, and SilenceUsage stops it
	// dumping the full usage for *every* error. Usage is still shown for
	// genuine parameter problems — flag parse errors are re-tagged here via
	// FlagErrorFunc, and espbrew's own parameter checks use usageErrf — while
	// runtime failures (device not found, connection refused, ...) print only
	// the error message.
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		usageCmd = cmd
		return usageErrf("%s", err)
	})

	// Global output-control flags (persistent: inherited by every subcommand).
	// These let espbrew behave like every other GitHub tool in CI while keeping
	// the rich interactive experience for a human at a console. See ci.go.
	f := rootCmd.PersistentFlags()
	f.BoolVar(&ciOpts.forceCI, "ci", false, "Force CI mode (concise output, GitHub annotations, no progress bar)")
	f.BoolVar(&ciOpts.forceInter, "interactive", false, "Force interactive mode (override CI auto-detection)")
	f.BoolVarP(&ciOpts.quiet, "quiet", "q", false, "Reduce output (implies CI-style logging)")
	f.BoolVarP(&ciOpts.verbose, "verbose", "v", false, "Verbose output (more Debug lines, even in CI)")
	f.StringVar(&ciOpts.logLevel, "log-level", "info", "Log level: debug, info, warn, error")

	// Resolve the global log level once flags are parsed (before any command
	// body runs) so flash/monitor/cluster all honour --ci/--quiet/--verbose.
	// Also record which command is running so printTopLevelError can print that
	// command's usage for parameter/usage errors (see usageCmd).
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		usageCmd = cmd
		zerolog.SetGlobalLevel(resolveLogLevel())
		return nil
	}
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
	clusterCmd.Flags().IntVar(&cfg.workers, "workers", 2, "Number of flash workers")
	clusterCmd.Flags().BoolVar(&cfg.disablemDNS, "no-mdns", false, "Disable mDNS discovery")
	clusterCmd.Flags().BoolVar(&cfg.devMode, "dev-mode", false, "Enable developer mode (unsafe for production)")
}

// usageCmd is the command whose UsageString is printed alongside a
// parameter/usage error. It defaults to the root command and is overridden to
// the offending subcommand (see FlagErrorFunc and the usageErrf call sites) so
// the most specific usage is shown.
var usageCmd = rootCmd

// usageError marks a CLI parameter/usage problem (bad flag, missing firmware,
// unknown preset, ...) so the top-level handler prints usage/help alongside the
// message. Runtime failures (device not found, connection refused, flash
// failure, ...) are plain errors and must NOT be tagged, so they never trigger
// a usage dump.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// usageErrf builds a parameter/usage error carrying the given message.
func usageErrf(format string, args ...interface{}) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

func main() {
	// Set up console logging for all commands. The level itself is resolved after
	// flag parsing in PersistentPreRunE (see ci.go / resolveLogLevel) so the
	// --ci/--quiet/--verbose/--log-level flags are honoured for every command.
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})

	if err := rootCmd.Execute(); err != nil {
		printTopLevelError(err)
		os.Exit(1)
	}
}

// printTopLevelError prints the error returned by the CLI. Usage/help is shown
// only for parameter problems (bad flag, missing firmware, unknown preset, ...);
// runtime failures print just the error message so a failed flash does not dump
// the man page onto the log.
func printTopLevelError(err error) {
	out := os.Stderr
	fmt.Fprintln(out, rootCmd.ErrPrefix(), err)

	var ue *usageError
	if errors.As(err, &ue) {
		fmt.Fprintln(out)
		fmt.Fprint(out, usageCmd.UsageString())
		return
	}

	// Unknown command / subcommand: nudge the user toward help.
	if strings.Contains(strings.ToLower(err.Error()), "unknown command") {
		fmt.Fprintln(out)
		fmt.Fprint(out, fmt.Sprintf("Run '%v --help' for usage.\n", rootCmd.CommandPath()))
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	level := resolveLogLevel()
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
