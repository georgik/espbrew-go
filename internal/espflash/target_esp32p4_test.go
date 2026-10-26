package espflash

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// revWord encodes a chip revision as major*100+minor in the EFUSE register
// layout used by chipRevision (major=((w>>23)&1)<<2|((w>>4)&3), minor=w&0xF).
// revWord(301) == 0x31: bits[5:4]=11 -> major 3, bits[3:0]=0001 -> minor 1.
func revWord(major, minor int) uint32 {
	return (uint32(minor) & 0x0F) | (uint32(major&0x07) << 4)
}

func TestESP32P4PostConnectUSBRev3(t *testing.T) {
	writeCount := 0

	mc := &mockConnection{
		readRegFunc: func(addr uint32) (uint32, error) {
			switch addr {
			case esp32p4EFUSE_REV_WORD:
				return revWord(3, 1), nil // v3.1 -> revision 301
			case esp32p4UARTDevBufNoRev3:
				return esp32p4UARTDevBufNoUSBJTAGSerial, nil // USB-JTAG/Serial
			default:
				return 0, nil
			}
		},
		writeRegFunc: func(addr, value, mask, delayUS uint32) error {
			writeCount++
			return nil
		},
	}
	f := &Flasher{conn: mc, opts: &FlasherOptions{}}

	err := esp32p4PostConnect(f)
	require.NoError(t, err)
	assert.True(t, f.usesUSB, "usesUSB should be true for USB-JTAG/Serial")
	assert.Greater(t, writeCount, 0, "should disable watchdogs and power on flash")
}

func TestESP32P4PostConnectUARTRev3(t *testing.T) {
	writeCount := 0

	mc := &mockConnection{
		readRegFunc: func(addr uint32) (uint32, error) {
			switch addr {
			case esp32p4EFUSE_REV_WORD:
				return revWord(3, 1), nil // v3.1 -> revision 301
			case esp32p4UARTDevBufNoRev3:
				return 0, nil // not USB-JTAG/Serial
			default:
				return 0, nil
			}
		},
		writeRegFunc: func(addr, value, mask, delayUS uint32) error {
			writeCount++
			return nil
		},
	}
	f := &Flasher{conn: mc, opts: &FlasherOptions{}}

	err := esp32p4PostConnect(f)
	require.NoError(t, err)
	assert.False(t, f.usesUSB, "usesUSB should be false for UART")
	assert.Greater(t, writeCount, 0, "power-on flash should still run on rev 3.0")
}

func TestESP32P4PostConnectRev1(t *testing.T) {
	// rev < 3.0: reads the rev1 UARTDEV_BUF_NO address and does NOT power on
	// the flash (flash power-on is only for rev 3.1/3.2).
	writeCount := 0

	mc := &mockConnection{
		readRegFunc: func(addr uint32) (uint32, error) {
			switch addr {
			case esp32p4EFUSE_REV_WORD:
				return revWord(1, 0), nil // v1.0 -> revision 100
			case esp32p4UARTDevBufNoRev1:
				return esp32p4UARTDevBufNoUSBJTAGSerial, nil
			default:
				return 0, nil
			}
		},
		writeRegFunc: func(addr, value, mask, delayUS uint32) error {
			writeCount++
			return nil
		},
	}
	f := &Flasher{conn: mc, opts: &FlasherOptions{}}

	err := esp32p4PostConnect(f)
	require.NoError(t, err)
	assert.True(t, f.usesUSB, "usesUSB should be true for USB-JTAG/Serial")
	// Only the watchdog-disable writes should happen (no power-on flash).
	assert.Greater(t, writeCount, 0)
}

func TestESP32P4PostConnectSecureMode(t *testing.T) {
	mc := &mockConnection{
		readRegFunc: func(addr uint32) (uint32, error) {
			return 0, errors.New("register not readable") // simulate secure mode
		},
	}
	f := &Flasher{conn: mc, opts: &FlasherOptions{}}

	err := esp32p4PostConnect(f)
	require.NoError(t, err, "should gracefully handle read error")
	assert.False(t, f.usesUSB, "should default to non-USB on read error")
}

func TestChipRevision(t *testing.T) {
	// revWord(3,1) must decode back to 301.
	f := &Flasher{
		conn: &mockConnection{
			readRegFunc: func(addr uint32) (uint32, error) {
				return revWord(3, 1), nil
			},
		},
	}
	assert.Equal(t, 301, chipRevision(f))

	// Unreadable register defaults to revision 0.
	f2 := &Flasher{
		conn: &mockConnection{
			readRegFunc: func(addr uint32) (uint32, error) {
				return 0, errors.New("boom")
			},
		},
	}
	assert.Equal(t, 0, chipRevision(f2))
}
