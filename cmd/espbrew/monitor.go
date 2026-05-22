package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.bug.st/serial"
	"golang.org/x/term"

	"github.com/spf13/cobra"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor --port <port>",
	Short: "Open serial monitor",
	RunE:  runMonitorCmd,
}

var monitorOpts struct {
	port   string
	baud   int
	exitOn string
}

func init() {
	monitorCmd.Flags().StringVarP(&monitorOpts.port, "port", "p", "", "Serial port (required)")
	monitorCmd.Flags().IntVar(&monitorOpts.baud, "baud", 115200, "Baud rate")
	monitorCmd.Flags().StringVar(&monitorOpts.exitOn, "exit-on", "", "Exit when string found in output")

	rootCmd.AddCommand(monitorCmd)
}

func runMonitorCmd(cmd *cobra.Command, args []string) error {
	if monitorOpts.port == "" {
		return fmt.Errorf("--port is required")
	}

	return runMonitor(monitorOpts.port, monitorOpts.baud)
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

	stdinFd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return fmt.Errorf("set raw terminal: %w", err)
	}
	defer term.Restore(stdinFd, oldState)

	fmt.Printf("Serial monitor on %s @ %d baud\n", portName, baud)
	fmt.Println("CTRL+R to reset, CTRL+C to exit")
	fmt.Println("---")

	exitCh := make(chan struct{}, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		exitCh <- struct{}{}
	}()

	buf := make([]byte, 1024)
	stdinBuf := make([]byte, 1)
	exitPattern := monitorOpts.exitOn

	for {
		select {
		case <-exitCh:
			fmt.Println("\r\nExiting monitor...")
			return nil

		default:
			n, err := serialPort.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
				if exitPattern != "" {
					if contains(string(buf[:n]), exitPattern) {
						fmt.Printf("\r\nExit pattern matched: %s\n", exitPattern)
						return nil
					}
				}
			}

			n, err = os.Stdin.Read(stdinBuf)
			if n > 0 {
				c := stdinBuf[0]
				if c == 18 { // CTRL+R - reset
					fmt.Println("\r\n[Resetting device - close/reopen port]")
					// Simple reset: DTR toggle via port reopen
					serialPort.Close()
					time.Sleep(100 * time.Millisecond)
					serialPort, err = serial.Open(portName, mode)
					if err != nil {
						return fmt.Errorf("reopen: %w", err)
					}
					serialPort.SetReadTimeout(50 * time.Millisecond)
					fmt.Println("---")
				} else if c == 3 { // CTRL+C
					fmt.Println("\r\nExiting...")
					return nil
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
