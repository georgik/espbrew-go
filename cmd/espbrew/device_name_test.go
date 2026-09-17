package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/georgik/espbrew-go/internal/inventory"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetCobraFlags clears the accumulated pflag "Changed" state so that repeated
// rootCmd.Execute() calls in a single test process behave like fresh runs
// (production invokes Execute() exactly once per process).
func resetCobraFlags() {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
		c.PersistentFlags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(rootCmd)
}

// execCLI drives the real command tree exactly as main.go does (rootCmd.Execute),
// which is the only path that correctly parses command flags.
func execCLI(t *testing.T, args ...string) {
	t.Helper()
	resetCobraFlags()
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func setTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	os.Stdout = old
	_ = w.Close()
	r.Seek(0, io.SeekStart)

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// TestDeviceSetUpdateName verifies `device set <id> --name` persists the name.
func TestDeviceSetUpdateName(t *testing.T) {
	setTempHome(t)

	inv, err := inventory.NewInventory()
	require.NoError(t, err)
	require.NoError(t, inv.Save(&inventory.DeviceInventory{
		DeviceID: "esp-84:f7:03:12:34:56", MACAddress: "84:f7:03:12:34:56", ChipType: "ESP32-S3",
		Tags: []string{}, Aliases: []string{},
	}))

	execCLI(t, "device", "set", "esp-84:f7:03:12:34:56", "--name", "living-room-brick")

	rel, err := inventory.NewInventory()
	require.NoError(t, err)
	got, err := rel.Get("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "living-room-brick", got.Name)
}

// TestDeviceSetClearName verifies `device set <id> --name ""` clears the name.
func TestDeviceSetClearName(t *testing.T) {
	setTempHome(t)

	inv, err := inventory.NewInventory()
	require.NoError(t, err)
	require.NoError(t, inv.Save(&inventory.DeviceInventory{
		DeviceID: "esp-84:f7:03:12:34:56", MACAddress: "84:f7:03:12:34:56", ChipType: "ESP32-S3",
		Name: "clear-me", Tags: []string{}, Aliases: []string{},
	}))

	execCLI(t, "device", "set", "esp-84:f7:03:12:34:56", "--name", "")

	rel, err := inventory.NewInventory()
	require.NoError(t, err)
	got, err := rel.Get("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "", got.Name)
}

// TestDeviceSetNameUnchangedWhenFlagAbsent verifies a name set earlier stays
// when `--name` is not passed on a later set, while unrelated flags still apply.
func TestDeviceSetNameUnchangedWhenFlagAbsent(t *testing.T) {
	setTempHome(t)

	inv, err := inventory.NewInventory()
	require.NoError(t, err)
	require.NoError(t, inv.Save(&inventory.DeviceInventory{
		DeviceID: "esp-84:f7:03:12:34:56", MACAddress: "84:f7:03:12:34:56", ChipType: "ESP32-S3",
		Name: "keep-me", Tags: []string{}, Aliases: []string{},
	}))

	execCLI(t, "device", "set", "esp-84:f7:03:12:34:56", "--model", "ESP32-S3-BOX-3")

	rel, err := inventory.NewInventory()
	require.NoError(t, err)
	got, err := rel.Get("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "keep-me", got.Name)
	assert.Equal(t, "ESP32-S3-BOX-3", got.BoardModel)
}

// TestDeviceShowIncludesName verifies `device show` prints the name.
func TestDeviceShowIncludesName(t *testing.T) {
	setTempHome(t)

	inv, err := inventory.NewInventory()
	require.NoError(t, err)
	require.NoError(t, inv.Save(&inventory.DeviceInventory{
		DeviceID: "esp-84:f7:03:12:34:56", MACAddress: "84:f7:03:12:34:56", ChipType: "ESP32-S3",
		Name: "living-room-brick", Tags: []string{}, Aliases: []string{},
	}))

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"device", "show", "esp-84:f7:03:12:34:56"})
		err := rootCmd.Execute()
		require.NoError(t, err)
	})
	assert.Contains(t, out, "living-room-brick")
}

// TestDeviceListIncludesName verifies `device list` renders the NAME column value.
func TestDeviceListIncludesName(t *testing.T) {
	setTempHome(t)

	inv, err := inventory.NewInventory()
	require.NoError(t, err)
	require.NoError(t, inv.Save(&inventory.DeviceInventory{
		DeviceID: "esp-84:f7:03:12:34:56", MACAddress: "84:f7:03:12:34:56", ChipType: "ESP32-S3",
		Name: "living-room-brick", Tags: []string{}, Aliases: []string{},
	}))

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"device", "list"})
		err := rootCmd.Execute()
		require.NoError(t, err)
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "living-room-brick")
}
