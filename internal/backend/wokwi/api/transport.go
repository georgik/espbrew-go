package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	// DefaultTimeout for operations
	DefaultTimeout = 30 * time.Second
	// DefaultWriteWait is time to wait for write operations
	DefaultWriteWait = 10 * time.Second
	// DefaultReadWait is time to wait for read operations (ping interval)
	DefaultReadWait = 60 * time.Second
	// DefaultPingInterval is the interval between pings
	DefaultPingInterval = 54 * time.Second
)

// Transport handles WebSocket communication with Wokwi API
type Transport struct {
	token     string
	url       string
	conn      *websocket.Conn
	mu        sync.Mutex
	nextID    int
	responses map[string]chan ResponseMessage
	eventSubs map[string][]chan EventMessage
	closeChan chan struct{}
	closed    bool
	userAgent string
}

// NewTransport creates a new WebSocket transport
func NewTransport(token, url string) *Transport {
	if url == "" {
		url = "wss://wokwi.com/api/ws/beta"
	}
	return &Transport{
		token:     token,
		url:       url,
		responses: make(map[string]chan ResponseMessage),
		eventSubs: make(map[string][]chan EventMessage),
		closeChan: make(chan struct{}),
		userAgent: "espbrew-go/1.0",
	}
}

// Connect establishes WebSocket connection and performs handshake
func (t *Transport) Connect(ctx context.Context) (*HelloMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn != nil {
		return nil, fmt.Errorf("already connected")
	}

	headers := make(map[string][]string)
	headers["Authorization"] = []string{"Bearer " + t.token}
	headers["User-Agent"] = []string{t.userAgent}

	dialer := websocket.Dialer{
		HandshakeTimeout: DefaultTimeout,
	}

	conn, _, err := dialer.DialContext(ctx, t.url, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	t.conn = conn
	t.closed = false

	// Set ping/pong handlers
	t.conn.SetPingHandler(nil)
	t.conn.SetPongHandler(nil)

	// Start message reader
	go t.readLoop()

	// Wait for hello message
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	helloCh := make(chan EventMessage, 1)
	t.eventSubs[MsgTypeHello] = append(t.eventSubs[MsgTypeHello], helloCh)

	select {
	case event := <-helloCh:
		if event.Type != MsgTypeHello {
			return nil, fmt.Errorf("expected hello, got %s", event.Type)
		}
		if protocolVersion, ok := event.Result["protocolVersion"].(float64); !ok || int(protocolVersion) != ProtocolVersion {
			return nil, fmt.Errorf("unsupported protocol version: %v", event.Result["protocolVersion"])
		}
		return &HelloMessage{
			Type:            event.Type,
			AppVersion:      stringOrEmpty(event.Result["appVersion"]),
			ProtocolVersion: int(event.Result["protocolVersion"].(float64)),
		}, nil
	case <-ctx.Done():
		t.Close()
		return nil, ctx.Err()
	}
}

// Close closes the WebSocket connection
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true
	close(t.closeChan)

	if t.conn != nil {
		// Send close message
		t.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		t.conn.Close()
		t.conn = nil
	}

	// Close all response channels
	for _, ch := range t.responses {
		close(ch)
	}
	t.responses = nil

	// Close all event subscriptions
	for _, subs := range t.eventSubs {
		for _, ch := range subs {
			close(ch)
		}
	}
	t.eventSubs = nil

	return nil
}

// Request sends a command and waits for response
func (t *Transport) Request(ctx context.Context, command string, params map[string]interface{}) (*ResponseMessage, error) {
	t.mu.Lock()
	if t.closed || t.conn == nil {
		t.mu.Unlock()
		return nil, fmt.Errorf("not connected")
	}

	id := fmt.Sprintf("%d", t.nextID)
	t.nextID++

	respChan := make(chan ResponseMessage, 1)
	t.responses[id] = respChan
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.responses, id)
		t.mu.Unlock()
	}()

	msg := Message{
		ID:      id,
		Type:    MsgTypeCommand,
		Command: command,
		Params:  params,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	t.mu.Lock()
	err = t.conn.SetWriteDeadline(time.Now().Add(DefaultWriteWait))
	if err != nil {
		t.mu.Unlock()
		return nil, fmt.Errorf("failed to set write deadline: %w", err)
	}
	err = t.conn.WriteMessage(websocket.TextMessage, data)
	t.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return &resp, fmt.Errorf("server error: %s", resp.Error.Message)
		}
		return &resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.closeChan:
		return nil, fmt.Errorf("connection closed")
	}
}

// Subscribe subscribes to events of a given type
func (t *Transport) Subscribe(eventType string) chan EventMessage {
	t.mu.Lock()
	defer t.mu.Unlock()

	ch := make(chan EventMessage, 10)
	t.eventSubs[eventType] = append(t.eventSubs[eventType], ch)
	return ch
}

// Unsubscribe removes an event subscription
func (t *Transport) Unsubscribe(eventType string, ch chan EventMessage) {
	t.mu.Lock()
	defer t.mu.Unlock()

	subs := t.eventSubs[eventType]
	for i, sub := range subs {
		if sub == ch {
			t.eventSubs[eventType] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(t.eventSubs[eventType]) == 0 {
		delete(t.eventSubs, eventType)
	}
}

// readLoop reads messages from WebSocket and dispatches them
func (t *Transport) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("readLoop panic")
		}
	}()

	for {
		t.mu.Lock()
		if t.closed || t.conn == nil {
			t.mu.Unlock()
			return
		}
		t.mu.Unlock()

		_, message, err := t.conn.ReadMessage()
		if err != nil {
			if !t.closed {
				log.Error().Err(err).Msg("read error")
				t.Close()
			}
			return
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(message, &raw); err != nil {
			log.Error().Err(err).Msg("failed to parse message")
			continue
		}

		msgType := stringOrEmpty(raw["type"])

		switch msgType {
		case MsgTypeHello:
			t.dispatchEvent(EventMessage{
				Type:   msgType,
				Result: raw,
			})
		case MsgTypeEvent:
			t.dispatchEvent(EventMessage{
				Type:   msgType,
				Event:  stringOrEmpty(raw["event"]),
				Nanos:  int64OrZero(raw["nanos"]),
				Result: getMap(raw["result"]),
				Data:   getMap(raw["data"]),
			})
		case MsgTypeResponse:
			t.handleResponse(raw)
		case MsgTypeError:
			log.Error().Str("message", stringOrEmpty(raw["message"])).Msg("server error")
		default:
			log.Debug().Str("type", msgType).Msg("unknown message type")
		}
	}
}

// handleResponse handles a response message
func (t *Transport) handleResponse(raw map[string]interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := stringOrEmpty(raw["id"])
	respChan, ok := t.responses[id]
	if !ok {
		return
	}

	resp := ResponseMessage{
		ID:     id,
		Type:   stringOrEmpty(raw["type"]),
		Result: getMap(raw["result"]),
	}

	if errData, ok := raw["error"].(map[string]interface{}); ok {
		resp.Error = &ErrorInfo{
			Code:    intOrZero(errData["code"]),
			Message: stringOrEmpty(errData["message"]),
		}
	}

	select {
	case respChan <- resp:
	default:
		log.Warn().Str("id", id).Msg("response channel full")
	}
}

// dispatchEvent dispatches an event to subscribers
func (t *Transport) dispatchEvent(event EventMessage) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var subs []chan EventMessage
	if event.Type == MsgTypeHello {
		subs = t.eventSubs[MsgTypeHello]
	} else {
		subs = t.eventSubs[event.Event]
	}

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			log.Warn().Str("event", event.Event).Msg("event channel full")
		}
	}
}

// Helper functions
func stringOrEmpty(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intOrZero(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}

func int64OrZero(v interface{}) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	if i, ok := v.(int); ok {
		return int64(i)
	}
	if i64, ok := v.(int64); ok {
		return i64
	}
	return 0
}

func getMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}
