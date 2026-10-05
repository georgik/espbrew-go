package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/georgik/espbrew-go/internal/persistence"
)

// DeviceConfig is a single entry from the espbrew.toml [[devices]] section.
// It is the explicit, user-authored source of truth for which physical port
// maps to which device identity. Auto-discovery is disabled by default; the
// watcher only reports whether a port is present, so the identity of a port
// must come from here or from an explicit, on-demand probe.
type DeviceConfig struct {
	Path        string `mapstructure:"path" toml:"path"`
	ID          string `mapstructure:"id" toml:"id"`
	ChipType    string `mapstructure:"chip" toml:"chip"`
	Alias       string `mapstructure:"alias" toml:"alias"`
	Description string `mapstructure:"description" toml:"description"`
	// Disabled marks a port that espbrew.toml intentionally excludes from
	// flashing (e.g. a companion USB-UART console on the board). Such a port is
	// recorded and shown in the API as disabled, and flash skips it, but it is
	// never given a usable flashing identity.
	Disabled bool `mapstructure:"disabled" toml:"disabled"`
	// USBLocation is the USB hub location (sysfs "location" string, e.g. "1-2")
	// that supplies power to this board. It is only meaningful on platforms
	// with a power-controllable USB hub (Linux, kernel >= 6.0). When set, espbrew
	// treats the board as power-managed: if the board is not present in the live
	// USB tree it is marked "sleeping"; an operation wakes it (powers the hub
	// port on) before starting, and the board is powered back off once it has
	// been idle for the configured timeout. See USBPort for the port number.
	USBLocation string `mapstructure:"usb_location" toml:"usb_location"`
	// USBPort is the 1-based port number on the hub at USBLocation that the board
	// is plugged into. Together with USBLocation it fully specifies where the
	// board sits in the USB tree, which is what espbrew needs to switch its power.
	USBPort int `mapstructure:"usb_port" toml:"usb_port"`
	// Camera is the stable reference of the camera whose view frames this
	// device. It may be the discovered camera ID (e.g. "cam-usb-…") or the
	// camera's stable Name. Set together with CameraBox so that, on startup and
	// on every device event, the leader persists a device→camera bounding-box
	// mapping (see internal/persistence). 'snap' then crops to this device
	// without any UI calibration. When empty, no mapping is written and 'snap'
	// falls back to the first discovered camera.
	Camera string `mapstructure:"camera" toml:"camera"`
	// CameraBox is the normalized bounding box (each value 0.0–1.0) that frames
	// this device within Camera's view. It is written to the bounding-box store
	// together with Camera. A fully-zero box is treated as "no box configured".
	CameraBox CameraBoxConfig `mapstructure:"camera_box" toml:"camera_box"`
}

// CameraBoxConfig is a normalized bounding box (0.0–1.0) relative to an image.
// It mirrors persistence.BoundingBox but lives in the config package so the
// espbrew.toml schema does not depend on the persistence layer.
type CameraBoxConfig struct {
	X      float64 `mapstructure:"x" toml:"x"`
	Y      float64 `mapstructure:"y" toml:"y"`
	Width  float64 `mapstructure:"width" toml:"width"`
	Height float64 `mapstructure:"height" toml:"height"`
}

// Bounded reports whether the box carries a non-trivial region (any side set).
func (b CameraBoxConfig) Bounded() bool {
	return b.X != 0 || b.Y != 0 || b.Width != 0 || b.Height != 0
}

// ToPersistence converts the normalized config box into the persistence model.
func (b CameraBoxConfig) ToPersistence() persistence.BoundingBox {
	return persistence.BoundingBox{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height}
}

// StringID is a stable key for a device config (path, since ports are the
// physical truth the watcher reports on).
func (d DeviceConfig) StringID() string { return d.Path }

type StaticPeerConfig struct {
	ID      string `mapstructure:"id"`
	Address string `mapstructure:"address"`
	Port    int    `mapstructure:"port"`
}

type ClusterConfig struct {
	ClusterName       string             `mapstructure:"cluster_name"`
	Role              string             `mapstructure:"role"` // leader, peer, standalone
	BindAddress       string             `mapstructure:"bind_address"`
	HTTPPort          int                `mapstructure:"http_port"`
	LeaderAddress     string             `mapstructure:"leader_address"` // For peers
	HeartbeatInterval time.Duration      `mapstructure:"heartbeat_interval"`
	NodeTimeout       time.Duration      `mapstructure:"node_timeout"`
	LogLevel          string             `mapstructure:"log_level"`
	StaticPeers       []StaticPeerConfig `mapstructure:"static_peers"`
	PeerDiscoveryMode string             `mapstructure:"peer_discovery"`         // "mdns", "static", "both"
	LeaderCandidates  []string           `mapstructure:"leader_candidates"`      // For HA
	Devices           []DeviceConfig     `mapstructure:"devices" toml:"devices"` // Explicit port->device mapping
}

// DeviceByPath returns the device config for a given port, or nil.
func (c *ClusterConfig) DeviceByPath(path string) *DeviceConfig {
	for i := range c.Devices {
		if c.Devices[i].Path == path {
			return &c.Devices[i]
		}
	}
	return nil
}

// PowerManaged reports whether the device is attached to a power-controllable
// hub (i.e. espbrew can switch its power on/off). A device is power-managed only
// when it declares a USB hub location.
func (d DeviceConfig) PowerManaged() bool {
	return strings.TrimSpace(d.USBLocation) != ""
}

// ValidatePower returns a non-nil error when a device declares a USB location
// but omits the port number required to switch its power. A device without a
// USB location is never power-managed and needs no port.
func (d DeviceConfig) ValidatePower() error {
	if !d.PowerManaged() {
		return nil
	}
	if d.USBPort < 1 {
		return fmt.Errorf("usb_location %q set without usb_port (port must be >= 1)", d.USBLocation)
	}
	return nil
}

// PowerLocation returns the hub location and 1-based port for a power-managed
// device. The second return value is false when the device is not
// power-managed.
func (d DeviceConfig) PowerLocation() (string, int, bool) {
	if !d.PowerManaged() {
		return "", 0, false
	}
	return d.USBLocation, d.USBPort, true
}

func Default() *ClusterConfig {
	return &ClusterConfig{
		ClusterName:       "espbrew-cluster",
		Role:              "standalone",
		BindAddress:       "0.0.0.0",
		HTTPPort:          8080,
		LeaderAddress:     "",
		HeartbeatInterval: 5 * time.Second,
		NodeTimeout:       30 * time.Second,
		LogLevel:          "info",
		PeerDiscoveryMode: "mdns",
	}
}
