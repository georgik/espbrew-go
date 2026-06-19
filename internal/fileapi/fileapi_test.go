//go:build js
// +build js

package fileapi

import (
	"reflect"
	"syscall/js"
	"testing"
)

func TestBrowserSupport(t *testing.T) {
	support := checkBrowserSupport()

	if support.InputFiles != true {
		t.Error("InputFiles should always be supported when document exists")
	}

	// FileSystemAccessAPI depends on browser
	// WebkitDirectory attribute is always available
	if !support.WebkitDirectory {
		t.Error("WebkitDirectory should be supported")
	}
}

func TestNewFolderPicker(t *testing.T) {
	picker := NewFolderPicker()
	if picker == nil {
		t.Fatal("NewFolderPicker returned nil")
	}

	if picker.supported.InputFiles != true {
		t.Error("Picker should support input files")
	}
}

func TestFolderFiles(t *testing.T) {
	files := &FolderFiles{
		Name:  "test-project",
		Files: make(map[string][]byte),
	}

	if files.Name != "test-project" {
		t.Errorf("expected Name 'test-project', got '%s'", files.Name)
	}

	if files.Files == nil {
		t.Error("Files map should be initialized")
	}

	// Test adding files
	files.Files["test.bin"] = []byte{0x01, 0x02, 0x03}
	if len(files.Files["test.bin"]) != 3 {
		t.Error("Failed to store file data")
	}
}

func TestConvertArrayBufferToBytes(t *testing.T) {
	// Create a test ArrayBuffer
	testData := []byte{0x01, 0x02, 0x03, 0x04}

	uint8Array := js.Global().Get("Uint8Array").New(len(testData))
	js.CopyBytesToJS(uint8Array, testData)
	arrayBuffer := uint8Array.Get("buffer")

	result := convertArrayBufferToBytes(arrayBuffer)

	if !reflect.DeepEqual(result, testData) {
		t.Errorf("expected %v, got %v", testData, result)
	}
}

func TestConvertArrayBufferToBytesNil(t *testing.T) {
	result := convertArrayBufferToBytes(js.Undefined())
	if result != nil {
		t.Errorf("expected nil for undefined input, got %v", result)
	}

	result = convertArrayBufferToBytes(js.Null())
	if result != nil {
		t.Errorf("expected nil for null input, got %v", result)
	}
}

func TestConvertArrayBufferToBytesEmpty(t *testing.T) {
	// Create empty ArrayBuffer
	uint8Array := js.Global().Get("Uint8Array").New(0)
	arrayBuffer := uint8Array.Get("buffer")

	result := convertArrayBufferToBytes(arrayBuffer)
	if result != nil {
		t.Errorf("expected nil for empty buffer, got %v", result)
	}
}

func TestFileError(t *testing.T) {
	err := &FileError{Message: "test error"}
	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got '%s'", err.Error())
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect string
	}{
		{"NoFilesSelected", ErrNoFilesSelected, "no files selected"},
		{"InvalidPromise", ErrInvalidPromise, "invalid promise"},
		{"NotAPromise", ErrNotAPromise, "not a promise"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.expect {
				t.Errorf("expected '%s', got '%s'", tt.expect, tt.err.Error())
			}
		})
	}
}
