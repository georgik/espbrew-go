// Command wokping dials a Wokwi server with the real espbrew API client and
// prints the hello handshake result. Used to validate protocol compatibility
// against a self-hosted wokwi-ci-server. Temporary probe.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/georgik/espbrew-go/internal/backend/wokwi/api"
)

func main() {
	url := flag.String("url", "", "Wokwi WS server URL, e.g. ws://host:3000/api/ws/beta")
	token := flag.String("token", "", "Wokwi CLI token")
	flag.Parse()

	if *url == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "usage: wokping -url ws://host:3000/api/ws/beta -token wok_...")
		os.Exit(2)
	}

	c := api.NewClientWithServer(*token, *url)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	fmt.Fprintln(os.Stderr, "[probe] connecting to "+*url+" ...")
	hello, err := c.Connect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[probe] CONNECT FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("CONNECT OK  appVersion=%s protocolVersion=%d url=%s\n", hello.AppVersion, hello.ProtocolVersion, *url)
	_ = c.Close()
	fmt.Fprintln(os.Stderr, "[probe] done")
	// Force exit even if anything lingers.
	os.Exit(0)
}
