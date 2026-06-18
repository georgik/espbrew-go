//go:build js
// +build js

package api

import (
	"strconv"
	"syscall/js"
)

// MonitorWebSocket manages a WebSocket connection to the serial monitor
type MonitorWebSocket struct {
	conn      js.Value
	mock      *mockMonitorWebSocket
	onMessage func(data string)
	onError   func(err error)
	onClose   func()
	connected bool
}

// MonitorConfig configures a monitor connection
type MonitorConfig struct {
	Port     string
	BaudRate int
	ExitOn   string
	Reset    bool
}

// NewMonitorWebSocket creates a new monitor WebSocket connection
func NewMonitorWebSocket(config *MonitorConfig) *MonitorWebSocket {
	// In demo mode, return mock monitor
	if DemoModeEnabled() {
		mockWS := NewMockMonitorWebSocket(config)
		return &MonitorWebSocket{
			conn:      js.Value{},
			mock:      mockWS,
			connected: true,
		}
	}

	mw := &MonitorWebSocket{}

	// Build WebSocket URL
	wsProtocol := "ws:"
	if js.Global().Get("location").Get("protocol").String() == "https:" {
		wsProtocol = "wss:"
	}
	host := js.Global().Get("location").Get("host").String()

	// Build URL with query parameters
	url := wsProtocol + "//" + host + "/api/v1/monitor/" + config.Port + "?baud=" + strconv.Itoa(config.BaudRate)
	if config.ExitOn != "" {
		url += "&exit_on=" + config.ExitOn
	}
	if config.Reset {
		url += "&reset=1"
	}

	// Create WebSocket
	conn := js.Global().Get("WebSocket").New(url)
	mw.conn = conn

	// Set up event handlers
	conn.Set("onmessage", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 && mw.onMessage != nil {
			event := args[0]
			rawData := event.Get("data").String()

			// Safely parse JSON to extract data field
			// Backend sends: {"data":"...","type":"data"}
			// Use JavaScript Function constructor for try-catch
			parseFunc := js.Global().Get("Function").New(
				"data",
				"try { return JSON.parse(data); } catch(e) { return null; }",
			)
			jsonObj := parseFunc.Invoke(rawData)

			if !jsonObj.IsNull() && !jsonObj.IsUndefined() && !jsonObj.Get("data").IsUndefined() {
				dataField := jsonObj.Get("data")
				mw.onMessage(dataField.String())
			} else {
				// If JSON parse fails or no data field, use raw data
				mw.onMessage(rawData)
			}
		}
		return nil
	}))

	conn.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if mw.onError != nil {
			mw.onError(&MonitorError{Message: "WebSocket error"})
		}
		return nil
	}))

	conn.Set("onclose", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		mw.connected = false
		if mw.onClose != nil {
			mw.onClose()
		}
		return nil
	}))

	conn.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		mw.connected = true
		return nil
	}))

	return mw
}

// SetMessageHandler sets the message handler
func (mw *MonitorWebSocket) SetMessageHandler(handler func(string)) {
	if mw.mock != nil {
		mw.mock.SetMessageHandler(handler)
		return
	}
	mw.onMessage = handler
}

// SetErrorHandler sets the error handler
func (mw *MonitorWebSocket) SetErrorHandler(handler func(error)) {
	if mw.mock != nil {
		mw.mock.SetErrorHandler(handler)
		return
	}
	mw.onError = handler
}

// SetCloseHandler sets the close handler
func (mw *MonitorWebSocket) SetCloseHandler(handler func()) {
	if mw.mock != nil {
		mw.mock.SetCloseHandler(handler)
		return
	}
	mw.onClose = handler
}

// Send sends data to the serial port
func (mw *MonitorWebSocket) Send(data string) error {
	if !mw.connected {
		return &MonitorError{Message: "Not connected"}
	}

	if mw.mock != nil {
		return mw.mock.Send(data)
	}

	mw.conn.Call("send", data)
	return nil
}

// Close closes the WebSocket connection
func (mw *MonitorWebSocket) Close() {
	if mw.mock != nil {
		mw.mock.Close()
		mw.connected = false
		return
	}

	if !mw.conn.IsNull() && !mw.conn.IsUndefined() {
		mw.conn.Call("close")
	}
	mw.connected = false
}

// IsConnected returns the connection status
func (mw *MonitorWebSocket) IsConnected() bool {
	return mw.connected
}

// MonitorError represents a monitor error
type MonitorError struct {
	Message string
}

func (e *MonitorError) Error() string {
	return e.Message
}
