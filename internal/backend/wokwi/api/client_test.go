package api

import (
	"encoding/json"
	"testing"
	"time"
)

// TestNewClient tests creating a new client
func TestNewClient(t *testing.T) {
	client := NewClient("test-token")
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	if client.transport == nil {
		t.Error("Expected transport to be initialized")
	}

	if client.transport.token != "test-token" {
		t.Errorf("Expected token 'test-token', got '%s'", client.transport.token)
	}
}

// TestNewClientWithServer tests creating a client with custom server
func TestNewClientWithServer(t *testing.T) {
	customURL := "wss://custom.wokwi.com/api/ws/beta"
	client := NewClientWithServer("test-token", customURL)
	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	if client.transport.url != customURL {
		t.Errorf("Expected URL '%s', got '%s'", customURL, client.transport.url)
	}
}

// TestSetDiagram tests setting the diagram
func TestSetDiagram(t *testing.T) {
	client := NewClient("test-token")
	diagram := `{"version": 1, "parts": []}`

	client.SetDiagram(diagram)

	if client.diagram != diagram {
		t.Errorf("Expected diagram to be set")
	}
}

// TestSetFirmware tests setting the firmware path
func TestSetFirmware(t *testing.T) {
	client := NewClient("test-token")
	firmware := "/path/to/firmware.bin"

	client.SetFirmware(firmware)

	if client.firmware != firmware {
		t.Errorf("Expected firmware path to be set")
	}
}

// TestSetELF tests setting the ELF path
func TestSetELF(t *testing.T) {
	client := NewClient("test-token")
	elf := "/path/to/firmware.elf"

	client.SetELF(elf)

	if client.elf != elf {
		t.Errorf("Expected ELF path to be set")
	}
}

// TestFlashSection tests FlashSection creation
func TestFlashSection(t *testing.T) {
	section := FlashSection{
		Offset: 0x1000,
		File:   "bootloader.bin",
	}

	if section.Offset != 0x1000 {
		t.Errorf("Expected offset 0x1000, got %d", section.Offset)
	}

	if section.File != "bootloader.bin" {
		t.Errorf("Expected file 'bootloader.bin', got '%s'", section.File)
	}
}

// TestIdfFirmwareUploadResult tests IdfFirmwareUploadResult creation
func TestIdfFirmwareUploadResult(t *testing.T) {
	flashSize := 4 * 1024 * 1024 // 4MB
	sections := []FlashSection{
		{Offset: 0x1000, File: "bootloader.bin"},
		{Offset: 0x8000, File: "partition-table.bin"},
		{Offset: 0x10000, File: "app.bin"},
	}

	result := IdfFirmwareUploadResult{
		Firmware:  sections,
		FlashSize: &flashSize,
	}

	if len(result.Firmware) != 3 {
		t.Errorf("Expected 3 sections, got %d", len(result.Firmware))
	}

	if result.FlashSize == nil || *result.FlashSize != flashSize {
		t.Errorf("Expected flash size %d", flashSize)
	}
}

// TestSimStartParams tests SimStartParams creation
func TestSimStartParams(t *testing.T) {
	params := SimStartParams{
		Firmware:  "firmware.bin",
		Elf:       "firmware.elf",
		Pause:     false,
		FlashSize: nil,
	}

	if params.Firmware != "firmware.bin" {
		t.Errorf("Expected firmware 'firmware.bin'")
	}

	if params.Elf != "firmware.elf" {
		t.Errorf("Expected ELF 'firmware.elf'")
	}

	if params.Pause {
		t.Error("Expected pause to be false")
	}
}

// TestMessageTypes tests message type constants
func TestMessageTypes(t *testing.T) {
	if MsgTypeHello != "hello" {
		t.Errorf("Expected MsgTypeHello to be 'hello'")
	}

	if MsgTypeCommand != "command" {
		t.Errorf("Expected MsgTypeCommand to be 'command'")
	}

	if MsgTypeResponse != "response" {
		t.Errorf("Expected MsgTypeResponse to be 'response'")
	}

	if MsgTypeEvent != "event" {
		t.Errorf("Expected MsgTypeEvent to be 'event'")
	}

	if MsgTypeError != "error" {
		t.Errorf("Expected MsgTypeError to be 'error'")
	}

	if ProtocolVersion != 1 {
		t.Errorf("Expected ProtocolVersion to be 1")
	}
}

// TestHelloMessage tests HelloMessage creation
func TestHelloMessage(t *testing.T) {
	hello := &HelloMessage{
		Type:            "hello",
		AppVersion:      "1.0.0",
		ProtocolVersion: 1,
	}

	if hello.Type != "hello" {
		t.Errorf("Expected type 'hello'")
	}

	if hello.AppVersion != "1.0.0" {
		t.Errorf("Expected app version '1.0.0'")
	}

	if hello.ProtocolVersion != 1 {
		t.Errorf("Expected protocol version 1")
	}
}

// TestResponseMessage tests ResponseMessage creation
func TestResponseMessage(t *testing.T) {
	resp := ResponseMessage{
		ID:     "123",
		Type:   "response",
		Result: map[string]interface{}{"status": "ok"},
	}

	if resp.ID != "123" {
		t.Errorf("Expected ID '123'")
	}

	if resp.Type != "response" {
		t.Errorf("Expected type 'response'")
	}

	if resp.Result["status"] != "ok" {
		t.Errorf("Expected result status 'ok'")
	}
}

// TestEventMessage tests EventMessage creation
func TestEventMessage(t *testing.T) {
	event := EventMessage{
		Type:  "event",
		Event: "sim:serial",
		Nanos: 1000000,
		Data:  map[string]interface{}{"text": "output"},
	}

	if event.Type != "event" {
		t.Errorf("Expected type 'event'")
	}

	if event.Event != "sim:serial" {
		t.Errorf("Expected event 'sim:serial'")
	}

	if event.Nanos != 1000000 {
		t.Errorf("Expected nanos 1000000")
	}
}

// TestErrorInfo tests ErrorInfo creation and Error method
func TestErrorInfo(t *testing.T) {
	errInfo := &ErrorInfo{
		Code:    400,
		Message: "Bad request",
	}

	if errInfo.Code != 400 {
		t.Errorf("Expected code 400")
	}

	if errInfo.Message != "Bad request" {
		t.Errorf("Expected message 'Bad request'")
	}

	expectedError := "server error: Bad request"
	if errInfo.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, errInfo.Error())
	}
}

// TestMessageSerialization tests message JSON serialization
func TestMessageSerialization(t *testing.T) {
	msg := Message{
		ID:      "123",
		Type:    MsgTypeCommand,
		Command: "sim:start",
		Params: map[string]interface{}{
			"firmware": "test.bin",
			"pause":    false,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	var unmarshaled Message
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if unmarshaled.ID != msg.ID {
		t.Errorf("Expected ID '%s', got '%s'", msg.ID, unmarshaled.ID)
	}

	if unmarshaled.Command != msg.Command {
		t.Errorf("Expected command '%s', got '%s'", msg.Command, unmarshaled.Command)
	}
}

// TestContains tests the contains helper function
func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		haystack []byte
		needle   []byte
		expected bool
	}{
		{
			name:     "needle at start",
			haystack: []byte("hello world"),
			needle:   []byte("hello"),
			expected: true,
		},
		{
			name:     "needle in middle",
			haystack: []byte("hello world"),
			needle:   []byte("lo wo"),
			expected: true,
		},
		{
			name:     "needle at end",
			haystack: []byte("hello world"),
			needle:   []byte("world"),
			expected: true,
		},
		{
			name:     "needle not found",
			haystack: []byte("hello world"),
			needle:   []byte("goodbye"),
			expected: false,
		},
		{
			name:     "empty needle",
			haystack: []byte("hello world"),
			needle:   []byte(""),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.haystack, tt.needle)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestTransportTimeoutConstants tests timeout constants
func TestTransportTimeoutConstants(t *testing.T) {
	if DefaultTimeout != 30*time.Second {
		t.Errorf("Expected DefaultTimeout to be 30s")
	}

	if DefaultWriteWait != 10*time.Second {
		t.Errorf("Expected DefaultWriteWait to be 10s")
	}

	if DefaultReadWait != 60*time.Second {
		t.Errorf("Expected DefaultReadWait to be 60s")
	}

	if DefaultPingInterval != 54*time.Second {
		t.Errorf("Expected DefaultPingInterval to be 54s")
	}
}

// TestNewTransport tests creating a new transport
func TestNewTransport(t *testing.T) {
	transport := NewTransport("test-token", "")

	if transport == nil {
		t.Fatal("Expected non-nil transport")
	}

	if transport.token != "test-token" {
		t.Errorf("Expected token 'test-token', got '%s'", transport.token)
	}

	if transport.url != "wss://wokwi.com/api/ws/beta" {
		t.Errorf("Expected default Wokwi URL")
	}
}

// TestNewTransportCustomURL tests creating a transport with custom URL
func TestNewTransportCustomURL(t *testing.T) {
	customURL := "wss://custom.example.com/ws"
	transport := NewTransport("test-token", customURL)

	if transport.url != customURL {
		t.Errorf("Expected URL '%s', got '%s'", customURL, transport.url)
	}
}
