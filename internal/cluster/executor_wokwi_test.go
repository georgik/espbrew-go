package cluster

import (
	"errors"
	"testing"
)

// TestExecutorWokwiHookSkipsSerialFlash verifies that when the executor's Wokwi
// hook reports handled=true, executeFlash returns immediately without attempting
// a serial flash (which is what lets a Wokwi device "flash" by starting a
// simulation instead of writing over a serial port).
func TestExecutorWokwiHookSkipsSerialFlash(t *testing.T) {
	e := NewJobExecutor(1)

	// A hook that handles the job means the serial flash must be skipped. Even
	// though the firmware file does not exist, executeFlash must not error
	// because it never reads it.
	e.wokwiHook = func(*Job) (bool, error) { return true, nil }

	job := &Job{Firmware: "/nonexistent/firmware.bin", DevicePath: "wokwi-1"}
	if err := e.executeFlash(job); err != nil {
		t.Fatalf("executeFlash with handled Wokwi hook: got error %v, want nil", err)
	}
}

// TestExecutorWokwiHookDelegatesToSerialFlash verifies that when the hook
// reports handled=false, executeFlash proceeds to the normal serial flash path
// (which errors here because the firmware file is absent).
func TestExecutorWokwiHookDelegatesToSerialFlash(t *testing.T) {
	e := NewJobExecutor(1)

	e.wokwiHook = func(*Job) (bool, error) { return false, nil }

	job := &Job{Firmware: "/nonexistent/firmware.bin", DevicePath: "ttyUSB0"}
	err := e.executeFlash(job)
	if err == nil {
		t.Fatal("executeFlash without handled hook: got nil, want error reading missing firmware")
	}
}

// TestExecutorWokwiHookPropagatesError verifies that an error returned by a
// handled hook is propagated as the job result.
func TestExecutorWokwiHookPropagatesError(t *testing.T) {
	e := NewJobExecutor(1)

	sentinel := errors.New("sim failed to start")
	e.wokwiHook = func(*Job) (bool, error) { return true, sentinel }

	job := &Job{Firmware: "/tmp/app.elf", DevicePath: "wokwi-1"}
	err := e.executeFlash(job)
	if !errors.Is(err, sentinel) {
		t.Fatalf("executeFlash: got error %v, want %v", err, sentinel)
	}
}
