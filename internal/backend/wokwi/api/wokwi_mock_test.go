package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// recordedCommand captures a command received from the client.
type recordedCommand struct {
	ID      string
	Command string
	Params  map[string]interface{}
}

// mockWokwiServer is an in-process Wokwi Simulation API server used to validate
// the client against the real wire contract. It speaks the exact protocol:
//
//   - sends a "hello" on connect,
//   - answers every "command" with a "response",
//   - can emit "event" messages (e.g. serial-monitor:data) to the client.
//
// Tests assert on the recorded commands (command names + params) and can drive
// events back to the client, giving real end-to-end coverage without the public
// wss://wokwi.com endpoint.
type mockWokwiServer struct {
	*httptest.Server

	mu           sync.Mutex
	recordedCmds []recordedCommand
	hello        HelloMessage
	respond      func(command string, params map[string]interface{}) (result map[string]interface{}, errMsg string, errCode int)

	// writeMu serializes writes because gorilla's *websocket.Conn is not
	// safe for concurrent use; readLoop (responses) and writeLoopConn
	// (events) both write from separate goroutines.
	writeMu sync.Mutex

	outCh chan interface{}
	done  chan struct{}
}

// newMockWokwiServer starts a mock server with a default success responder.
func newMockWokwiServer() *mockWokwiServer {
	m := &mockWokwiServer{
		hello: HelloMessage{
			Type:            MsgTypeHello,
			AppVersion:      "mock-1.0",
			ProtocolVersion: ProtocolVersion,
		},
		outCh: make(chan interface{}, 16),
		done:  make(chan struct{}),
	}
	m.Server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

// url returns the ws:// URL of the mock server.
func (m *mockWokwiServer) wsURL() string {
	return "ws" + m.Server.URL[len("http"):] + "/api/ws/beta"
}

// setResponder overrides how the server answers a command. Returning a non-empty
// errMsg makes the server reply with an error result.
func (m *mockWokwiServer) setResponder(respond func(command string, params map[string]interface{}) (result map[string]interface{}, errMsg string, errCode int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.respond = respond
}

// emitSerial sends a serial-monitor:data event with the given bytes.
func (m *mockWokwiServer) emitSerial(data []byte) {
	bytes := make([]interface{}, len(data))
	for i, b := range data {
		bytes[i] = int(b)
	}
	m.emitEvent("serial-monitor:data", map[string]interface{}{"bytes": bytes})
}

// emitEvent sends a generic event message.
func (m *mockWokwiServer) emitEvent(event string, payload map[string]interface{}) {
	msg := map[string]interface{}{
		"type":    MsgTypeEvent,
		"event":   event,
		"payload": payload,
		"nanos":   42,
	}
	m.pushOut(msg)
}

func (m *mockWokwiServer) pushOut(v interface{}) {
	select {
	case m.outCh <- v:
	case <-m.done:
	}
}

// commands returns a copy of the recorded commands.
func (m *mockWokwiServer) commands() []recordedCommand {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]recordedCommand, len(m.recordedCmds))
	copy(out, m.recordedCmds)
	return out
}

// findCommand returns the last recorded command with the given name, if any.
func (m *mockWokwiServer) findCommand(name string) (recordedCommand, bool) {
	cmds := m.commands()
	for i := len(cmds) - 1; i >= 0; i-- {
		if cmds[i].Command == name {
			return cmds[i], true
		}
	}
	return recordedCommand{}, false
}

// hasCommand reports whether any recorded command has the given name.
func (m *mockWokwiServer) hasCommand(name string) bool {
	_, ok := m.findCommand(name)
	return ok
}

// writeMsg writes v over conn, serializing with writeMu so concurrent
// readLoop/writeLoopConn goroutines never touch the conn at the same time.
func (m *mockWokwiServer) writeMsg(conn *websocket.Conn, v interface{}) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	_ = conn.WriteMessage(websocket.TextMessage, mustJSON(v))
}

func (m *mockWokwiServer) handle(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	m.writeMsg(conn, m.hello)
	go m.readLoop(conn)
	go m.writeLoopConn(conn)
}

func (m *mockWokwiServer) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var cmd struct {
			ID      string                 `json:"id"`
			Type    string                 `json:"type"`
			Command string                 `json:"command"`
			Params  map[string]interface{} `json:"params"`
		}
		if json.Unmarshal(data, &cmd) != nil {
			continue
		}
		if cmd.Type != MsgTypeCommand {
			continue
		}

		m.mu.Lock()
		m.recordedCmds = append(m.recordedCmds, recordedCommand{
			ID:      cmd.ID,
			Command: cmd.Command,
			Params:  cmd.Params,
		})
		m.mu.Unlock()

		result := map[string]interface{}{}
		errMsg := ""
		errCode := 0
		func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.respond != nil {
				result, errMsg, errCode = m.respond(cmd.Command, cmd.Params)
			}
		}()

		resp := map[string]interface{}{
			"type":    MsgTypeResponse,
			"command": cmd.Command,
			"id":      cmd.ID,
			"result":  result,
			"error":   errMsg != "",
		}
		if errMsg != "" {
			resp["error"] = map[string]interface{}{"code": errCode, "message": errMsg}
		}
		m.writeMsg(conn, resp)
	}
}

func (m *mockWokwiServer) writeLoopConn(conn *websocket.Conn) {
	for {
		select {
		case v := <-m.outCh:
			m.writeMsg(conn, v)
		case <-m.done:
			return
		}
	}
}

// mustJSON marshals v to JSON, panicking on error (only pure values are passed).
func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// stop closes the mock server and its websocket conns.
func (m *mockWokwiServer) stop() {
	close(m.done)
	m.Server.Close()
	// give in-flight writes a moment
	time.Sleep(10 * time.Millisecond)
}
