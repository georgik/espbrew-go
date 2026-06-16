package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.bug.st/serial"
	"golang.org/x/term"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/device"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// forceFlush forces stdout to flush immediately (works for terminals)
func forceFlush() {
	syscall.Fsync(int(os.Stdout.Fd()))
}

var monitorCmd = &cobra.Command{
	Use:   "monitor [flags]",
	Short: "Open serial monitor",
	RunE:  runMonitorCmd,
}

var monitorOpts struct {
	clusterURL  string
	port        string
	baud        int
	exitOn      string
	exitOnError string
	duration    int
	noRaw       bool
	resetFirst  bool
}

func init() {
	monitorCmd.Flags().StringVar(&monitorOpts.clusterURL, "cluster", "", "Cluster URL for remote monitoring")
	monitorCmd.Flags().StringVarP(&monitorOpts.port, "port", "p", "", "Serial port (auto-detect if empty)")
	monitorCmd.Flags().IntVar(&monitorOpts.baud, "baud", 115200, "Baud rate")
	monitorCmd.Flags().StringVar(&monitorOpts.exitOn, "exit-on", "", "Exit when string found (success)")
	monitorCmd.Flags().StringVar(&monitorOpts.exitOnError, "exit-on-error", "", "Exit when string found (failure, exit 1)")
	monitorCmd.Flags().IntVar(&monitorOpts.duration, "duration", 0, "Auto-exit after N seconds (0=no limit)")
	monitorCmd.Flags().BoolVar(&monitorOpts.noRaw, "no-raw", false, "Skip raw terminal mode (for testing)")
	monitorCmd.Flags().BoolVar(&monitorOpts.resetFirst, "reset", false, "Reset device before monitoring (captures boot logs)")

	rootCmd.AddCommand(monitorCmd)
}

func runMonitorCmd(cmd *cobra.Command, args []string) error {
	if monitorOpts.clusterURL != "" {
		return runMonitorRemote()
	}
	return runMonitorLocal()
}

func runMonitorRemote() error {
	client := cluster.NewClient(monitorOpts.clusterURL)

	// Get available devices if port not specified
	var devicePath string
	if monitorOpts.port == "" {
		devices, err := client.ListDevices()
		if err != nil {
			return fmt.Errorf("list devices: %w", err)
		}

		// Find first available device
		for _, d := range devices {
			if d.State == "available" {
				devicePath = d.Path
				break
			}
		}

		if devicePath == "" {
			return fmt.Errorf("no available devices on cluster")
		}

		log.Info().Str("device", devicePath).Msg("Auto-selected available device")
	} else {
		devicePath = monitorOpts.port
	}

	// Reserve device for monitoring
	clientID := "espbrew-monitor-" + randomID(8)
	monitorClient := cluster.NewMonitorClient(monitorOpts.clusterURL, devicePath, cluster.MonitorConfig{
		Baud:        monitorOpts.baud,
		Reset:       monitorOpts.resetFirst,
		ExitOn:      monitorOpts.exitOn,
		ExitOnError: monitorOpts.exitOnError,
		Duration:    time.Duration(monitorOpts.duration) * time.Second,
	})

	log.Info().Str("device", devicePath).Str("cluster", monitorOpts.clusterURL).Msg("Reserving device for monitoring")
	if err := monitorClient.ReserveDevice(clientID, 300); err != nil {
		log.Warn().Err(err).Msg("Could not reserve device (may already be reserved)")
	}
	defer func() {
		log.Info().Str("device", devicePath).Msg("Releasing device")
		monitorClient.ReleaseDevice(clientID)
	}()

	return runMonitorRemoteStream(monitorClient)
}

func runMonitorRemoteStream(monitorClient *cluster.MonitorClient) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer monitorClient.Close()

	dataCh := make(chan []byte, 256)
	errorCh := make(chan error, 1)

	if err := monitorClient.Stream(ctx, dataCh, errorCh); err != nil {
		return fmt.Errorf("connect monitor: %w", err)
	}

	// Create unbuffered writer for immediate output in raw mode
	stdoutWriter := &bufioWriter{w: os.Stdout}

	fmt.Printf("Remote monitor on %s @ %d baud\r\n", monitorOpts.port, monitorOpts.baud)
	if !monitorOpts.noRaw {
		fmt.Printf("CTRL+R to reset, CTRL+C to exit\r\n")
	}
	fmt.Printf("---\r\n")

	// Set up signal handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	exitCh := make(chan monitorExit, 1)

	go func() {
		select {
		case <-sigCh:
			exitCh <- monitorExit{success: true, message: "Interrupted"}
		case err := <-errorCh:
			if err != nil {
				exitCh <- monitorExit{success: false, message: err.Error()}
			}
		}
	}()

	// stdin reader for reset command
	var stdinBuf []byte
	var oldState *term.State
	if !monitorOpts.noRaw {
		stdinBuf = make([]byte, 1)
		var err error
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			oldState = nil
		}
		if oldState != nil {
			defer term.Restore(int(os.Stdin.Fd()), oldState)
		}
	}

	var timeoutCh <-chan time.Time
	if monitorOpts.duration > 0 {
		timeoutCh = time.After(time.Duration(monitorOpts.duration) * time.Second)
	}

	for {
		select {
		case exit := <-exitCh:
			// Restore terminal before printing
			if oldState != nil {
				term.Restore(int(os.Stdin.Fd()), oldState)
				oldState = nil
			}
			fmt.Printf("\n%s\n", exit.message)
			if exit.success {
				return nil
			}
			return fmt.Errorf("monitor failed: %s", exit.message)

		case <-timeoutCh:
			// Restore terminal before printing
			if oldState != nil {
				term.Restore(int(os.Stdin.Fd()), oldState)
				oldState = nil
			}
			fmt.Printf("\nDuration limit reached (%d seconds)\n", monitorOpts.duration)
			return nil

		case data, ok := <-dataCh:
			if !ok {
				return nil
			}
			log.Debug().Int("bytes", len(data)).Str("content", string(data)).Msg("Writing data to stdout")
			stdoutWriter.Write(data)
			stdoutWriter.Flush()

			// Check exit patterns
			dataStr := string(data)
			if monitorOpts.exitOnError != "" && contains(dataStr, monitorOpts.exitOnError) {
				exitCh <- monitorExit{success: false, message: fmt.Sprintf("Error pattern matched: %s", monitorOpts.exitOnError)}
			}
			if monitorOpts.exitOn != "" && contains(dataStr, monitorOpts.exitOn) {
				exitCh <- monitorExit{success: true, message: fmt.Sprintf("Success pattern matched: %s", monitorOpts.exitOn)}
			}

		default:
			if !monitorOpts.noRaw {
				n, _ := os.Stdin.Read(stdinBuf)
				if n > 0 {
					c := stdinBuf[0]
					if c == 18 { // CTRL+R
						fmt.Print("\r\n[Resetting device]\r\n")
						if err := monitorClient.Reset(); err != nil {
							log.Warn().Err(err).Msg("Reset failed")
						}
						fmt.Print("---\r\n")
					} else if c == 3 { // CTRL+C
						exitCh <- monitorExit{success: true, message: "Exiting..."}
					}
				}
			}
		}
	}
}

func runMonitorLocal() error {
	port := monitorOpts.port
	if port == "" {
		scanner := device.NewScanner()
		espPorts, err := scanner.ScanESP()
		if err != nil || len(espPorts) == 0 {
			return fmt.Errorf("--port required or no ESP devices found")
		}
		port = espPorts[0].Path
		fmt.Printf("Auto-detected ESP device: %s\n", port)
	}

	return runMonitor(port, monitorOpts.baud)
}

type monitorExit struct {
	success bool
	message string
}

func runMonitor(portName string, baud int) error {
	mode := &serial.Mode{
		BaudRate: baud,
	}

	serialPort, err := serial.Open(portName, mode)
	if err != nil {
		return fmt.Errorf("open serial port: %w", err)
	}
	defer serialPort.Close()

	serialPort.SetReadTimeout(50 * time.Millisecond)

	// Create unbuffered writer for immediate output in raw mode
	stdoutWriter := &bufioWriter{w: os.Stdout}

	// Start reader goroutine FIRST (before reset to capture boot logs)
	dataCh := make(chan []byte, 256)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := serialPort.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case dataCh <- data:
				default:
				}
			}
			if err != nil {
				close(dataCh)
				return
			}
		}
	}()

	// stdin reader for reset command
	var stdinBuf []byte
	var oldState *term.State
	if !monitorOpts.noRaw {
		stdinBuf = make([]byte, 1)
	}

	// Reset device AFTER reader is running (to capture boot logs)
	if monitorOpts.resetFirst {
		fmt.Println("[Resetting device to capture boot logs...]")
		serialPort.SetDTR(false)
		serialPort.SetRTS(true)
		time.Sleep(100 * time.Millisecond)
		serialPort.SetRTS(false)
		time.Sleep(50 * time.Millisecond)
		serialPort.SetDTR(false)
	}

	if !monitorOpts.noRaw {
		stdinFd := int(os.Stdin.Fd())
		var err error
		oldState, err = term.MakeRaw(stdinFd)
		if err != nil {
			return fmt.Errorf("set raw terminal: %w", err)
		}
		if oldState != nil {
			defer term.Restore(stdinFd, oldState)
		}
	}

	fmt.Printf("Serial monitor on %s @ %d baud\n", portName, baud)
	if !monitorOpts.noRaw {
		fmt.Println("CTRL+R to reset, CTRL+C to exit")
	}
	fmt.Println("---")

	exitCh := make(chan monitorExit, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		exitCh <- monitorExit{success: true, message: "Interrupted"}
	}()

	var timeoutCh <-chan time.Time
	if monitorOpts.duration > 0 {
		timeoutCh = time.After(time.Duration(monitorOpts.duration) * time.Second)
	}

	for {
		select {
		case exit := <-exitCh:
			// Restore terminal before printing
			if oldState != nil {
				term.Restore(int(os.Stdin.Fd()), oldState)
				oldState = nil
			}
			fmt.Printf("\n%s\n", exit.message)
			if exit.success {
				return nil
			}
			return fmt.Errorf("monitor failed: %s", exit.message)

		case <-timeoutCh:
			// Restore terminal before printing
			if oldState != nil {
				term.Restore(int(os.Stdin.Fd()), oldState)
				oldState = nil
			}
			fmt.Printf("\nDuration limit reached (%d seconds)\n", monitorOpts.duration)
			return nil

		case data, ok := <-dataCh:
			if !ok {
				return nil
			}
			dataStr := string(data)
			stdoutWriter.Write(data)
			stdoutWriter.Flush()

			if monitorOpts.exitOnError != "" && contains(dataStr, monitorOpts.exitOnError) {
				exitCh <- monitorExit{success: false, message: fmt.Sprintf("Error pattern matched: %s", monitorOpts.exitOnError)}
			}
			if monitorOpts.exitOn != "" && contains(dataStr, monitorOpts.exitOn) {
				exitCh <- monitorExit{success: true, message: fmt.Sprintf("Success pattern matched: %s", monitorOpts.exitOn)}
			}

		default:
			if !monitorOpts.noRaw {
				n, _ := os.Stdin.Read(stdinBuf)
				if n > 0 {
					c := stdinBuf[0]
					if c == 18 {
						fmt.Println("\r\n[Resetting device]")
						serialPort.SetDTR(false)
						serialPort.SetRTS(true)
						time.Sleep(100 * time.Millisecond)
						serialPort.SetRTS(false)
						time.Sleep(50 * time.Millisecond)
						fmt.Println("---")
					} else if c == 3 {
						exitCh <- monitorExit{success: true, message: "Exiting..."}
					}
				}
			}
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// bufioWriter wraps stdout with explicit flush for raw mode
type bufioWriter struct {
	w *os.File
}

func (b *bufioWriter) Write(p []byte) (n int, err error) {
	return b.w.Write(p)
}

func (b *bufioWriter) Flush() error {
	// Use syscall.Fsync on file descriptor for immediate terminal output
	return syscall.Fsync(int(b.w.Fd()))
}
