package main

import (
	"context"
	"fmt"
	"os"

	"github.com/georgik/espbrew-go/internal/espflash"
)

func main() {
	port := "/dev/ttyACM0"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	addr, size := uint32(0x0), uint32(0x4000)
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: probe_read <port> [addr]")
	}

	opts := espflash.DefaultOptions()
	opts.ChipType = espflash.ChipESP32P4
	opts.BaudRate = 115200
	opts.FlashBaudRate = 460800
	opts.FastMode = false
	opts.ConnectAttempts = 4

	f, err := espflash.New(context.Background(), port, opts)
	if err != nil {
		fmt.Printf("connect FAILED: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	data, err := f.ReadFlash(addr, size)
	if err != nil {
		fmt.Printf("read FAILED: %v\n", err)
		os.Exit(1)
	}
	zeroFF := 0
	for _, b := range data {
		if b == 0xFF {
			zeroFF++
		}
	}
	fmt.Printf("port=%s addr=0x%06x size=%d bytes\n", port, addr, len(data))
	fmt.Printf("0xFF bytes: %d/%d (%.1f%%)\n", zeroFF, len(data), 100*float64(zeroFF)/float64(len(data)))
	erased := zeroFF == len(data)
	fmt.Printf("ERASED=%v\n", erased)
	// first 32 bytes hex
	if len(data) >= 32 {
		fmt.Printf("first32: %x\n", data[:32])
	}
	if !erased {
		os.Exit(2)
	}
}
