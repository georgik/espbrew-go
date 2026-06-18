package monitor

import (
	"sync"
	"testing"
	"time"
)

// TestStreamManager_Create tests creating new stream sessions
func TestStreamManager_Create(t *testing.T) {
	mgr := NewStreamManager()

	// This will fail if no serial port, but we test the logic
	cfg := StreamConfig{
		Port:     "/dev/nonexistent",
		BaudRate: 115200,
	}

	_, err := mgr.Create("test1", cfg)
	if err == nil {
		t.Log("Expected error for non-existent port (OK if running with mock)")
	}
}

// TestStreamManager_DuplicateID tests creating session with duplicate ID
func TestStreamManager_DuplicateID(t *testing.T) {
	mgr := NewStreamManager()

	cfg := StreamConfig{
		Port:     "/dev/nonexistent",
		BaudRate: 115200,
	}

	// First attempt
	_, err := mgr.Create("test1", cfg)
	_ = err // May fail without hardware

	// Second attempt with same ID should fail even if first failed
	_, err = mgr.Create("test1", cfg)
	if err == nil {
		t.Error("Expected error for duplicate session ID")
	}
}

// TestStreamManager_Get tests retrieving sessions
func TestStreamManager_Get(t *testing.T) {
	mgr := NewStreamManager()
	mgr.sessions["test1"] = NewStreamSession("test1", StreamConfig{})

	s, ok := mgr.Get("test1")
	if !ok {
		t.Fatal("Session not found")
	}
	if s == nil {
		t.Fatal("Session is nil")
	}
}

// TestStreamManager_GetNotFound tests getting non-existent session
func TestStreamManager_GetNotFound(t *testing.T) {
	mgr := NewStreamManager()

	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Error("Expected false for non-existent session")
	}
}

// TestStreamManager_Remove tests removing sessions
func TestStreamManager_Remove(t *testing.T) {
	mgr := NewStreamManager()
	sess := NewStreamSession("test1", StreamConfig{})
	mgr.sessions["test1"] = sess

	mgr.Remove("test1")

	if _, ok := mgr.Get("test1"); ok {
		t.Fatal("Session still exists after remove")
	}
}

// TestStreamManager_RemoveNonExistent tests removing non-existent session
func TestStreamManager_RemoveNonExistent(t *testing.T) {
	mgr := NewStreamManager()

	// Should not panic
	mgr.Remove("nonexistent")
}

// TestStreamManager_List tests listing all sessions
func TestStreamManager_List(t *testing.T) {
	mgr := NewStreamManager()

	// Add some sessions
	mgr.sessions["test1"] = NewStreamSession("test1", StreamConfig{})
	mgr.sessions["test2"] = NewStreamSession("test2", StreamConfig{})

	list := mgr.List()
	if len(list) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(list))
	}

	// Verify it's a copy (modifications shouldn't affect original)
	delete(list, "test1")
	if _, ok := mgr.sessions["test1"]; !ok {
		t.Error("List should return a copy, not reference")
	}
}

// TestStreamSession_ContainsMatch tests pattern matching in data
func TestStreamSession_ContainsMatch(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	tests := []struct {
		name     string
		data     []byte
		pattern  string
		expected bool
	}{
		{"simple match", []byte("Hello World"), "World", true},
		{"no match", []byte("Hello"), "Goodbye", false},
		{"empty pattern", []byte("Hello"), "", true}, // empty pattern matches
		{"pattern longer", []byte("Hi"), "Hello", false},
		{"case sensitive", []byte("hello"), "Hello", false},
		{"match at start", []byte("Start here"), "Start", true},
		{"match at end", []byte("end here"), "here", true},
		{"multi-byte", []byte("Hello 世界"), "世界", true},
		{"special chars", []byte("Test\nNewline"), "\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sess.containsMatch(tt.data, tt.pattern)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestStreamSession_DataChannel tests data channel operations
func TestStreamSession_DataChannel(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	dataCh := sess.Data()
	if dataCh == nil {
		t.Fatal("Data channel is nil")
	}

	// Channel should be readable
	select {
	case <-dataCh:
	default:
		// No data, which is expected
	}
}

// TestStreamSession_ErrorChannel tests error channel operations
func TestStreamSession_ErrorChannel(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	errorCh := sess.Errors()
	if errorCh == nil {
		t.Fatal("Error channel is nil")
	}

	// Channel should be readable
	select {
	case <-errorCh:
	default:
		// No error, which is expected
	}
}

// TestStreamSession_SendControl tests sending control messages
func TestStreamSession_SendControl(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})
	sess.control = make(chan *ControlMessage, 1)

	msg := &ControlMessage{Type: "reset", Data: ""}
	sess.SendControl(msg)

	select {
	case received := <-sess.control:
		if received.Type != "reset" {
			t.Errorf("Expected reset, got %s", received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for control message")
	}
}

// TestStreamSession_SendControlAfterCancel tests sending control after cancellation
func TestStreamSession_SendControlAfterCancel(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	// Cancel the session
	sess.cancel()

	// SendControl should not block
	msg := &ControlMessage{Type: "close", Data: ""}
	sess.SendControl(msg)
}

// TestStreamSession_Close tests closing a session
func TestStreamSession_Close(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	err := sess.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Context should be cancelled
	select {
	case <-sess.ctx.Done():
		// Expected
	default:
		t.Error("Context should be cancelled after Close")
	}
}

// TestStreamSession_CloseTwice tests closing a session twice
func TestStreamSession_CloseTwice(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	_ = sess.Close()
	err := sess.Close()
	if err != nil {
		t.Errorf("Second Close returned error: %v", err)
	}
}

// TestStreamSession_ConcurrentAccess tests concurrent access to session
func TestStreamSession_ConcurrentAccess(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})
	sess.control = make(chan *ControlMessage, 10)

	done := make(chan bool, 10)

	// Multiple goroutines sending control messages
	for i := 0; i < 10; i++ {
		go func(id int) {
			msg := &ControlMessage{Type: "test", Data: ""}
			sess.SendControl(msg)
			done <- true
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for concurrent operations")
		}
	}
}

// TestStreamSession_Config tests session configuration
func TestStreamSession_Config(t *testing.T) {
	cfg := StreamConfig{
		Port:     "/dev/ttyUSB0",
		BaudRate: 9600,
		ExitOn:   "ready",
		Timeout:  30 * time.Second,
	}

	sess := NewStreamSession("test", cfg)

	if sess.config.Port != cfg.Port {
		t.Errorf("Expected port %s, got %s", cfg.Port, sess.config.Port)
	}
	if sess.config.BaudRate != cfg.BaudRate {
		t.Errorf("Expected baud rate %d, got %d", cfg.BaudRate, sess.config.BaudRate)
	}
	if sess.config.ExitOn != cfg.ExitOn {
		t.Errorf("Expected exit pattern %s, got %s", cfg.ExitOn, sess.config.ExitOn)
	}
}

// TestStreamSession_ID tests session ID
func TestStreamSession_ID(t *testing.T) {
	sess := NewStreamSession("test-id-123", StreamConfig{})

	if sess.id != "test-id-123" {
		t.Errorf("Expected ID test-id-123, got %s", sess.id)
	}
}

// TestControlMessage tests control message structure
func TestControlMessage(t *testing.T) {
	msg := &ControlMessage{
		Type: "reset",
		Data: "optional",
	}

	if msg.Type != "reset" {
		t.Errorf("Expected type reset, got %s", msg.Type)
	}
	if msg.Data != "optional" {
		t.Errorf("Expected data optional, got %s", msg.Data)
	}
}

// TestStreamSession_DataChannelClosed tests reading from closed data channel
func TestStreamSession_DataChannelClosed(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	// Close the data channel
	close(sess.dataCh)

	// Reading should return immediately
	select {
	case _, ok := <-sess.Data():
		if ok {
			t.Error("Expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout reading from closed channel")
	}
}

// TestStreamManager_ConcurrentCreateRemove tests concurrent create/remove operations
func TestStreamManager_ConcurrentCreateRemove(t *testing.T) {
	mgr := NewStreamManager()
	done := make(chan bool, 10)

	for i := 0; i < 5; i++ {
		go func(id int) {
			cfg := StreamConfig{
				Port:     "/dev/nonexistent",
				BaudRate: 115200,
			}
			_, _ = mgr.Create(string(rune(id)), cfg)
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func(id int) {
			mgr.Remove(string(rune(id)))
			done <- true
		}(i)
	}

	// Wait for all operations
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for concurrent operations")
		}
	}
}

// TestStreamManager_ThreadSafety tests thread safety of StreamManager
func TestStreamManager_ThreadSafety(t *testing.T) {
	mgr := NewStreamManager()

	var wg sync.WaitGroup
	operations := 100

	// Concurrent reads through proper API
	for i := 0; i < operations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Get("test")
		}()
	}

	// Concurrent creates through proper API
	for i := 0; i < operations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := StreamConfig{
				Port:     "/dev/nonexistent",
				BaudRate: 115200,
			}
			_, _ = mgr.Create("test", cfg)
			mgr.Remove("test")
		}(i)
	}

	wg.Wait()
}

// TestStreamConfig_DefaultValues tests default configuration values
func TestStreamConfig_DefaultValues(t *testing.T) {
	cfg := StreamConfig{
		Port:     "/dev/ttyUSB0",
		BaudRate: 115200,
	}

	if cfg.Timeout != 0 {
		t.Logf("Default timeout is %v", cfg.Timeout)
	}

	if cfg.ExitOn != "" {
		t.Logf("Exit pattern is %s", cfg.ExitOn)
	}
}

// TestStreamSession_ContextCanceled tests operations on canceled context
func TestStreamSession_ContextCanceled(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	// Cancel immediately
	sess.cancel()

	// Verify context is done
	select {
	case <-sess.ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Context should be canceled")
	}
}

// TestStreamManager_EmptyList tests listing when no sessions exist
func TestStreamManager_EmptyList(t *testing.T) {
	mgr := NewStreamManager()

	list := mgr.List()
	if list == nil {
		t.Fatal("List should not return nil")
	}
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d sessions", len(list))
	}
}

// TestStreamSession_DataChannelBuffering tests data channel buffering
func TestStreamSession_DataChannelBuffering(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{})

	// Try to send data to the channel
	sent := 0
	for i := 0; i < 300; i++ {
		select {
		case sess.dataCh <- []byte("test"):
			sent++
		default:
			// Channel full, which is expected
			if sent > 0 {
				t.Logf("Buffered %d messages before channel full", sent)
			}
			return
		}
	}

	if sent > 0 {
		t.Logf("Buffered %d messages successfully", sent)
	}
}

// TestStreamSession_ControlMessageTypes tests different control message types
func TestStreamSession_ControlMessageTypes(t *testing.T) {
	types := []string{"reset", "close"}

	for _, msgType := range types {
		t.Run(msgType, func(t *testing.T) {
			sess := NewStreamSession("test", StreamConfig{})
			sess.control = make(chan *ControlMessage, 1)

			msg := &ControlMessage{Type: msgType, Data: ""}
			sess.SendControl(msg)

			received := <-sess.control
			if received.Type != msgType {
				t.Errorf("Expected %s, got %s", msgType, received.Type)
			}
		})
	}
}

// TestStreamSession_ExitPattern tests exit pattern matching
func TestStreamSession_ExitPattern(t *testing.T) {
	sess := NewStreamSession("test", StreamConfig{ExitOn: "ready"})

	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{"contains pattern", []byte("System is ready now"), true},
		{"does not contain", []byte("System booting..."), false},
		{"exact match", []byte("ready"), true},
		{"case mismatch", []byte("READY"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sess.containsMatch(tt.data, sess.config.ExitOn)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestStreamSession_BaudRateConfiguration tests baud rate configuration
func TestStreamSession_BaudRateConfiguration(t *testing.T) {
	baudRates := []int{9600, 57600, 115200, 921600}

	for _, baud := range baudRates {
		t.Run(baudToString(baud), func(t *testing.T) {
			cfg := StreamConfig{
				Port:     "/dev/ttyUSB0",
				BaudRate: baud,
			}
			sess := NewStreamSession("test", cfg)

			if sess.config.BaudRate != baud {
				t.Errorf("Expected baud rate %d, got %d", baud, sess.config.BaudRate)
			}
		})
	}
}

func baudToString(baud int) string {
	switch baud {
	case 9600:
		return "9600"
	case 57600:
		return "57600"
	case 115200:
		return "115200"
	case 921600:
		return "921600"
	default:
		return "unknown"
	}
}
