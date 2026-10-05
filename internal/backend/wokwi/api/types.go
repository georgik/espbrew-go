package api

import "fmt"

// Protocol message types
const (
	ProtocolVersion = 1
	MsgTypeHello    = "hello"
	MsgTypeCommand  = "command"
	MsgTypeResponse = "response"
	MsgTypeEvent    = "event"
	MsgTypeError    = "error"
)

// Message represents a generic message in the Wokwi protocol
type Message struct {
	ID      string                 `json:"id,omitempty"`
	Type    string                 `json:"type"`
	Command string                 `json:"command,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty"`
	Error   *ErrorInfo             `json:"error,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
}

// HelloMessage is sent by the server on connection
type HelloMessage struct {
	Type            string `json:"type"`
	AppVersion      string `json:"appVersion"`
	ProtocolVersion int    `json:"protocolVersion"`
	// ServerURL is the endpoint the client connected to (added client-side for logging).
	ServerURL string `json:"-"`
}

// ResponseMessage is the response to a command. Command echoes the requested
// command name so the client can correlate responses.
type ResponseMessage struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Command string                 `json:"command,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *ErrorInfo             `json:"error,omitempty"`
}

// EventMessage is an unsolicited event from the server.
//
// The Wokwi protocol carries event data under the "payload" key, e.g. a
// "serial-monitor:data" event is:
//
//	{ "type": "event", "event": "serial-monitor:data",
//	  "payload": { "bytes": [10, 10], "nanos": 123 }, ... }
//
// Payload therefore holds that map. Result/Data are retained only for the
// legacy "hello" message (which ships its fields at the top level).
type EventMessage struct {
	Type    string                 `json:"type"`
	Event   string                 `json:"event"`
	Nanos   int64                  `json:"nanos,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// ErrorInfo represents an error in the protocol
type ErrorInfo struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message"`
}

// Error returns the error message
func (e *ErrorInfo) Error() string {
	return fmt.Sprintf("server error: %s", e.Message)
}

// FlashSection represents a section of firmware to flash
type FlashSection struct {
	Offset int    `json:"offset"`
	File   string `json:"file"`
	// Data is optional in-memory content. When set, UploadFirmwareSections
	// uploads it directly instead of reading File from disk. It is a purely
	// transient field and is never serialized on the wire (json:"-").
	Data []byte `json:"-"`
}

// IdfFirmwareUploadResult is the result of uploading ESP-IDF firmware
type IdfFirmwareUploadResult struct {
	Firmware  []FlashSection `json:"firmware"`
	FlashSize *int           `json:"flash_size,omitempty"`
}

// FlasherArgs represents the structure of flasher_args.json
type FlasherArgs struct {
	FlashParts     []FlashPart `json:"flash_parts"`
	FlashSizeBytes int         `json:"flash_size_bytes"`
	FlashMode      string      `json:"flash_mode"`
	FlashFreq      string      `json:"flash_freq"`
}

// FlashPart represents a single part in flasher_args.json
type FlashPart struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
}

// UploadParams represents parameters for file upload
type UploadParams struct {
	Name   string `json:"name"`
	Binary string `json:"binary"`
}

// SimStartParams represents parameters for sim:start command
type SimStartParams struct {
	Firmware  interface{} `json:"firmware,omitempty"` // string or []FlashSection
	Elf       string      `json:"elf,omitempty"`
	Chips     []string    `json:"chips,omitempty"`
	Pause     bool        `json:"pause,omitempty"`
	FlashSize *int        `json:"flash_size,omitempty"`
}
