package api

import (
	"context"
	"testing"
	"time"
)

// waitFor polls cond until it returns true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestTransportHandshake(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()

	tr := NewTransport("tok", srv.wsURL())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hello, err := tr.Connect(ctx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if hello.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocol version = %d, want %d", hello.ProtocolVersion, ProtocolVersion)
	}
	if hello.AppVersion != "mock-1.0" {
		t.Errorf("app version = %q, want mock-1.0", hello.AppVersion)
	}
	if hello.Type != MsgTypeHello {
		t.Errorf("hello type = %q, want %q", hello.Type, MsgTypeHello)
	}
}

func TestTransportRequestResponse(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()

	tr := NewTransport("tok", srv.wsURL())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := tr.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}

	resp, err := tr.Request(ctx, "sim:start", map[string]interface{}{"firmware": "flash-1000.bin"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.Command != "sim:start" {
		t.Errorf("command = %q, want sim:start", resp.Command)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %+v", resp.Error)
	}

	// The command must have reached the server with the right name/params.
	cmd, ok := srv.findCommand("sim:start")
	if !ok {
		t.Fatal("server did not receive sim:start")
	}
	if cmd.Params["firmware"] != "flash-1000.bin" {
		t.Errorf("firmware param = %v, want flash-1000.bin", cmd.Params["firmware"])
	}
}

func TestTransportServerError(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()
	srv.setResponder(func(command string, params map[string]interface{}) (map[string]interface{}, string, int) {
		return nil, "boom", 4001
	})

	tr := NewTransport("tok", srv.wsURL())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := tr.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}

	_, err := tr.Request(ctx, "sim:start", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if want := "server error: boom"; err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}

func TestTransportEventPayloadDispatch(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()

	tr := NewTransport("tok", srv.wsURL())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := tr.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}

	ch := tr.Subscribe("serial-monitor:data")
	defer tr.Unsubscribe("serial-monitor:data", ch)

	go srv.emitSerial([]byte("hello serial\n"))

	select {
	case ev := <-ch:
		if ev.Event != "serial-monitor:data" {
			t.Errorf("event = %q, want serial-monitor:data", ev.Event)
		}
		// The payload.bytes must survive the round-trip as integers.
		raw, ok := ev.Payload["bytes"]
		if !ok {
			t.Fatal("event payload has no 'bytes' key")
		}
		nums, ok := raw.([]interface{})
		if !ok || len(nums) != len("hello serial\n") {
			t.Fatalf("bytes payload = %#v, want %d numbers", raw, len("hello serial\n"))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for serial event")
	}
}

func TestTransportClose(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.stop()

	tr := NewTransport("tok", srv.wsURL())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := tr.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// A second close is a no-op.
	if err := tr.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
