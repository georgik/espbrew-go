package api

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// connectClient connects a client to the mock server.
func connectClient(t *testing.T, srv *mockWokwiServer) *Client {
	t.Helper()
	c := NewClientWithServer("tok", srv.wsURL())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return c
}

func TestNewClientDefaultURL(t *testing.T) {
	c := NewClient("tok")
	if c.transport.url != "wss://wokwi.com/api/ws/beta" {
		t.Errorf("default url = %q", c.transport.url)
	}
}

func TestNewClientCustomURL(t *testing.T) {
	c := NewClientWithServer("tok", "wss://custom.example.com/api/ws/beta")
	if c.transport.url != "wss://custom.example.com/api/ws/beta" {
		t.Errorf("url = %q", c.transport.url)
	}
}

func TestUploadFile(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	c := connectClient(t, srv)

	if err := c.UploadFile(context.Background(), "diagram.json", []byte(`{"version":1}`)); err != nil {
		t.Fatalf("upload: %v", err)
	}

	cmd, ok := srv.findCommand("file:upload")
	if !ok {
		t.Fatal("server did not receive file:upload")
	}
	if cmd.Params["name"] != "diagram.json" {
		t.Errorf("name = %v, want diagram.json", cmd.Params["name"])
	}
	decoded, err := base64.StdEncoding.DecodeString(cmd.Params["binary"].(string))
	if err != nil {
		t.Fatalf("binary not base64: %v", err)
	}
	if string(decoded) != `{"version":1}` {
		t.Errorf("decoded = %q, want {\"version\":1}", decoded)
	}
}

func TestUploadFirmwareSections(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()

	// Create two firmware files.
	dir := t.TempDir()
	boot := filepath.Join(dir, "bootloader.bin")
	app := filepath.Join(dir, "app.bin")
	if err := os.WriteFile(boot, []byte{0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app, []byte{0x03, 0x04, 0x05}, 0o644); err != nil {
		t.Fatal(err)
	}

	c := connectClient(t, srv)
	sections := []FlashSection{{Offset: 0x1000, File: boot}, {Offset: 0x10000, File: app}}
	uploaded, err := c.UploadFirmwareSections(context.Background(), sections)
	if err != nil {
		t.Fatalf("upload sections: %v", err)
	}
	if len(uploaded) != 2 {
		t.Fatalf("uploaded %d sections, want 2", len(uploaded))
	}
	if uploaded[0].File != "flash-1000.bin" {
		t.Errorf("section 0 remote name = %q, want flash-1000.bin", uploaded[0].File)
	}
	if uploaded[1].File != "flash-10000.bin" {
		t.Errorf("section 1 remote name = %q, want flash-10000.bin", uploaded[1].File)
	}

	// Two file:upload commands must have been recorded.
	uploads := 0
	for _, cmd := range srv.commands() {
		if cmd.Command == "file:upload" {
			uploads++
		}
	}
	if uploads != 2 {
		t.Errorf("file:upload count = %d, want 2", uploads)
	}
}

func TestParseFlasherArgs(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []struct {
		name string
		data []byte
	}{
		{"bootloader.bin", []byte{0x01}},
		{"partition-table.bin", []byte{0x02}},
		{"app.bin", []byte{0x03}},
	} {
		if err := os.WriteFile(filepath.Join(dir, f.name), f.data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	argsPath := filepath.Join(dir, "flasher_args.json")
	args := `{
		"flash_files": {
			"1000": "bootloader.bin",
			"8000": "partition-table.bin",
			"10000": "app.bin"
		},
		"flash_settings": { "flash_size": "4MB" }
	}`
	if err := os.WriteFile(argsPath, []byte(args), 0o644); err != nil {
		t.Fatal(err)
	}

	sections, flashSize, err := ParseFlasherArgs(argsPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sections) != 3 {
		t.Fatalf("sections = %d, want 3", len(sections))
	}
	// Offsets must be parsed as hex.
	if sections[0].Offset != 0x1000 || sections[1].Offset != 0x8000 || sections[2].Offset != 0x10000 {
		t.Errorf("offsets = [%d,%d,%d], want [0x1000,0x8000,0x10000]",
			sections[0].Offset, sections[1].Offset, sections[2].Offset)
	}
	// Paths must be resolved to absolute.
	if !filepath.IsAbs(sections[0].File) {
		t.Errorf("section path %q is not absolute", sections[0].File)
	}
	if flashSize == nil || *flashSize != 4*1024*1024 {
		t.Errorf("flashSize = %v, want %d", flashSize, 4*1024*1024)
	}
}

func TestUploadFirmwareFromFlasherArgs(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()

	dir := t.TempDir()
	for _, f := range []struct {
		name string
		data []byte
	}{
		{"bootloader.bin", []byte{0x01}},
		{"app.bin", []byte{0x03}},
	} {
		if err := os.WriteFile(filepath.Join(dir, f.name), f.data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	argsPath := filepath.Join(dir, "flasher_args.json")
	args := `{"flash_files":{"1000":"bootloader.bin","10000":"app.bin"},"flash_settings":{"flash_size":"1MB"}}`
	if err := os.WriteFile(argsPath, []byte(args), 0o644); err != nil {
		t.Fatal(err)
	}

	c := connectClient(t, srv)
	uploaded, flashSize, err := c.UploadFirmwareFromFlasherArgs(context.Background(), argsPath)
	if err != nil {
		t.Fatalf("upload from args: %v", err)
	}
	if len(uploaded) != 2 {
		t.Errorf("uploaded %d, want 2", len(uploaded))
	}
	if flashSize == nil || *flashSize != 1024*1024 {
		t.Errorf("flashSize = %v, want %d", flashSize, 1024*1024)
	}
}

func TestUploadELF(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()

	elfPath := filepath.Join(t.TempDir(), "firmware.elf")
	if err := os.WriteFile(elfPath, []byte{0x7f, 'E', 'L', 'F'}, 0o644); err != nil {
		t.Fatal(err)
	}

	c := connectClient(t, srv)
	name, err := c.UploadELF(context.Background(), elfPath)
	if err != nil {
		t.Fatalf("upload elf: %v", err)
	}
	if name != "firmware.elf" {
		t.Errorf("uploaded name = %q, want firmware.elf", name)
	}
	if _, ok := srv.findCommand("file:upload"); !ok {
		t.Fatal("server did not receive file:upload for ELF")
	}
}

func TestStartSimulationWithSections(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	c := connectClient(t, srv)

	sections := []FlashSection{{Offset: 0x1000, File: "flash-1000.bin"}}
	err := c.StartSimulation(context.Background(), SimStartParams{
		Firmware:  sections,
		Elf:       "firmware.elf",
		FlashSize: intPtr(4194304),
		Pause:     false,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	cmd, ok := srv.findCommand("sim:start")
	if !ok {
		t.Fatal("server did not receive sim:start")
	}
	// firmware must be an array of sections, not a bare string.
	_, isArr := cmd.Params["firmware"].([]interface{})
	if !isArr {
		t.Errorf("firmware param type = %T, want []interface{} (section array)", cmd.Params["firmware"])
	}
	if cmd.Params["elf"] != "firmware.elf" {
		t.Errorf("elf = %v, want firmware.elf", cmd.Params["elf"])
	}
	if cmd.Params["flashSize"] != float64(4194304) {
		t.Errorf("flashSize = %v, want 4194304", cmd.Params["flashSize"])
	}
}

func TestStartSimulationSingleFile(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	c := connectClient(t, srv)

	err := c.StartSimulation(context.Background(), SimStartParams{Firmware: "app.bin"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	cmd, _ := srv.findCommand("sim:start")
	if s, _ := cmd.Params["firmware"].(string); s != "app.bin" {
		t.Errorf("firmware = %v, want app.bin (string)", cmd.Params["firmware"])
	}
}

func TestSerialMonitorListen(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	c := connectClient(t, srv)

	if err := c.ListenSerial(context.Background()); err != nil {
		t.Fatalf("listen: %v", err)
	}
	if !srv.hasCommand("serial-monitor:listen") {
		t.Fatal("server did not receive serial-monitor:listen")
	}
}

func TestWriteSerial(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	c := connectClient(t, srv)

	if err := c.WriteSerial(context.Background(), []byte("AT\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd, ok := srv.findCommand("serial-monitor:write")
	if !ok {
		t.Fatal("server did not receive serial-monitor:write")
	}
	bytes, ok := cmd.Params["bytes"].([]interface{})
	if !ok || len(bytes) != 3 {
		t.Fatalf("bytes = %#v, want 3 numbers", cmd.Params["bytes"])
	}
	// "AT\n" = 65, 84, 10
	want := []int{65, 84, 10}
	for i, w := range want {
		if int(bytes[i].(float64)) != w {
			t.Errorf("bytes[%d] = %v, want %d", i, bytes[i], w)
		}
	}
}

func TestReadSerialBytes(t *testing.T) {
	c := NewClient("tok")
	ev := EventMessage{
		Event: "serial-monitor:data",
		Payload: map[string]interface{}{
			"bytes": []interface{}{float64('H'), float64('i'), float64('\n')},
		},
	}
	got := c.ReadSerialBytes(ev)
	if string(got) != "Hi\n" {
		t.Errorf("ReadSerialBytes = %q, want %q", got, "Hi\n")
	}
}

func TestBytesFromPayloadVariants(t *testing.T) {
	if got := BytesFromPayload(nil, "bytes"); got != nil {
		t.Errorf("nil payload => %q, want nil", got)
	}
	if got := BytesFromPayload(map[string]interface{}{}, "bytes"); got != nil {
		t.Errorf("missing key => %q, want nil", got)
	}
	got := BytesFromPayload(map[string]interface{}{
		"bytes": []interface{}{int(65), int(66), int(67)},
	}, "bytes")
	if string(got) != "ABC" {
		t.Errorf("int bytes => %q, want ABC", got)
	}
}

func TestWaitForOutput(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	c := connectClient(t, srv)

	if err := c.ListenSerial(context.Background()); err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Emit serial output in two chunks.
	go func() {
		time.Sleep(20 * time.Millisecond)
		srv.emitSerial([]byte("INFO - "))
		time.Sleep(20 * time.Millisecond)
		srv.emitSerial([]byte("Display initialized\n"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.WaitForOutput(ctx, "Display initialized", 3*time.Second); err != nil {
		t.Fatalf("wait for output: %v", err)
	}
}

func TestResumePauseRestartCommands(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	c := connectClient(t, srv)
	ctx := context.Background()

	if err := c.PauseSimulation(ctx); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := c.ResumeSimulation(ctx, nil); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := c.RestartSimulation(ctx, false); err != nil {
		t.Fatalf("restart: %v", err)
	}

	for _, name := range []string{"sim:pause", "sim:resume", "sim:restart"} {
		if !srv.hasCommand(name) {
			t.Errorf("server did not receive %q", name)
		}
	}
}

func TestStreamOutput(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	c := connectClient(t, srv)

	if err := c.ListenSerial(context.Background()); err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		srv.emitSerial([]byte("line1\nline2\n"))
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel shortly after the emitted data has arrived so StreamOutput returns.
	go func() {
		time.Sleep(60 * time.Millisecond)
		cancel()
	}()

	var sb strings.Builder
	if err := c.StreamOutput(ctx, &sb); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if sb.String() != "line1\nline2\n" {
		t.Errorf("stream output = %q, want line1\\nline2\\n", sb.String())
	}
}

func intPtr(i int) *int { return &i }
