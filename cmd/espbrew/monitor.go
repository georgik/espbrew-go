package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.bug.st/serial"
	"golang.org/x/term"

	"github.com/georgik/esp-ci-cluster/internal/device"
	"github.com/spf13/cobra"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor [flags]",
	Short: "Open serial monitor",
	RunE:  runMonitorCmd,
}

var monitorOpts struct {
	port        string
	baud        int
	exitOn      string
	exitOnError string
	duration    int
	noRaw       bool
	resetFirst  bool
}

func init() {
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
		oldState, err := term.MakeRaw(stdinFd)
		if err != nil {
			return fmt.Errorf("set raw terminal: %w", err)
		}
		defer term.Restore(stdinFd, oldState)
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

	var stdinBuf []byte
	if !monitorOpts.noRaw {
		stdinBuf = make([]byte, 1)
	}

	var timeoutCh <-chan time.Time
	if monitorOpts.duration > 0 {
		timeoutCh = time.After(time.Duration(monitorOpts.duration) * time.Second)
	}

	for {
		select {
		case exit := <-exitCh:
			fmt.Printf("\r\n%s\n", exit.message)
			if exit.success {
				return nil
			}
			return fmt.Errorf("monitor failed: %s", exit.message)

		case <-timeoutCh:
			fmt.Printf("\r\nDuration limit reached (%d seconds)\n", monitorOpts.duration)
			return nil

		case data, ok := <-dataCh:
			if !ok {
				return nil
			}
			dataStr := string(data)
			os.Stdout.Write(data)

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
