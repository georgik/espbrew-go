//go:build linux
// +build linux

package powercontrol

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func boardPresent() bool {
	out, _ := exec.Command("lsusb").Output()
	return strings.Contains(string(out), "303a")
}

func TestProbePowerOn(t *testing.T) {
	ctrl := NewController()
	hub, err := ctrl.FindHubByLocation("3-10")
	if err != nil {
		// Requires a real USB hub at location 3-10; skip on CI / bare machines.
		t.Skipf("no USB hub at 3-10 (requires real hardware): %v", err)
	}
	t.Logf("board present BEFORE power-on: %v", boardPresent())
	for i := 0; i < 3; i++ {
		err := ctrl.SetPortPower(hub, 1, true)
		t.Logf("SetPortPower(on) attempt %d: err=%v", i+1, err)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(3 * time.Second)
	t.Logf("board present AFTER power-on: %v", boardPresent())
}
