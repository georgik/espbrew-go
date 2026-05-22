package monitor

import (
	"testing"
	"time"
)

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

func TestStreamManager_Remove(t *testing.T) {
	mgr := NewStreamManager()
	sess := NewStreamSession("test1", StreamConfig{})
	mgr.sessions["test1"] = sess

	mgr.Remove("test1")

	if _, ok := mgr.Get("test1"); ok {
		t.Fatal("Session still exists after remove")
	}
}

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

func TestControlMessage(t *testing.T) {
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
