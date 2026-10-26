package espflash

import "time"

// ESP32-P4 register addresses for USB interface detection, watchdog control,
// and flash power-on. The target is revision-aware: rev < 3.0 (ECO2) and
// rev >= 3.0 (production silicon) differ in the UARTDEV_BUF_NO address and in
// the flash power-on requirement.
//
// Reference: esptool/targets/esp32p4.py (P4-specific values) and
// esptool/targets/esp32.py (ESP32ROM base-class SPI defaults P4 inherits).
//
// UARTDEV_BUF_NO is revision-dependent in esptool:
//
//	rev <  3.0 (ECO2):  0x4FF3FEB0 + 24 = 0x4FF3FEC8
//	rev >= 3.0 (prod):  0x4FFBFEB0 + 24 = 0x4FFBFEC8
//
// The USB-JTAG/Serial sentinel is 6 on P4 (not 3 like C5/C6/H2).
const (
	// esp32p4UARTDevBufNoRev1 is the UARTDEV_BUF_NO address for rev < 3.0.
	esp32p4UARTDevBufNoRev1 uint32 = 0x4FF3FEC8
	// esp32p4UARTDevBufNoRev3 is the UARTDEV_BUF_NO address for rev >= 3.0.
	esp32p4UARTDevBufNoRev3 uint32 = 0x4FFBFEC8
	// esp32p4UARTDevBufNoUSBJTAGSerial is the USB-JTAG/Serial sentinel.
	esp32p4UARTDevBufNoUSBJTAGSerial uint32 = 6

	// esp32p4EFUSE_REV_WORD is EFUSE_BLOCK1 word 2 (EFUSE_BASE+0x44 + 8),
	// which encodes the chip revision.
	esp32p4EFUSE_REV_WORD uint32 = 0x5012D044 + 8
	// esp32p4EFUSE_RD_REPEAT_DATA1 is used for the ECO7
	// DOWNLOAD_MODE_XPD_ON check.
	esp32p4EFUSE_RD_REPEAT_DATA1 uint32 = 0x5012D034

	// LP domain watchdog registers.
	esp32p4LPWDTConfig0     uint32 = 0x50116000
	esp32p4LPWDTWProtect    uint32 = 0x50116018
	esp32p4LPWDTSWDConf     uint32 = 0x5011601C
	esp32p4LPWDTSWDWProtect uint32 = 0x50116020

	// Flash power-on registers (rev >= 3.0 / ECO6-ECO7). DR_REG_LPAON_BASE =
	// 0x50110000, DR_REG_PMU_BASE = 0x50110000 + 0x5000.
	esp32p4LPSystemAnaXpdPadGroup uint32 = 0x50110000 + 0x10C
	esp32p4PMUAnaBase             uint32 = 0x50110000 + 0x5000
	esp32p4PMUExtLDOP0_0P1aAna    uint32 = 0x50110000 + 0x5000 + 0x1BC
	esp32p4PMUExtLDOP0_0P1a       uint32 = 0x50110000 + 0x5000 + 0x1B8
	esp32p4PMUDate                uint32 = 0x50110000 + 0x5000 + 0x3FC

	// Flash power-on bit fields.
	esp32p4PMUAna0P1aEnCurLim uint32 = 1 << 27
	// esp32p4PMU0P1aTarget0 is PMU_0P1A_TARGET0_0 (0xFF << 23).
	esp32p4PMU0P1aTarget0         uint32 = 0xFF << 23
	esp32p4PMU0P1aForceTieHSel    uint32 = 1 << 7
	esp32p4PMUDateFlashForceOn    uint32 = 0x3
	esp32p4EFUSEDownloadModeXPdon uint32 = 1 << 16
)

// ESP32-P4 target definition (revision-aware: rev1 and rev >= 3.0).
// Reference: https://github.com/espressif/esptool/blob/master/esptool/targets/esp32p4.py
var defESP32P4 = &chipDef{
	ChipType:       ChipESP32P4,
	Name:           "ESP32-P4",
	ImageChipID:    18,
	UsesMagicValue: false,

	SPIRegBase:  0x5008D000,
	SPIUSROffs:  0x18,
	SPIUSR1Offs: 0x1C,
	SPIUSR2Offs: 0x20,
	SPIMOSIOffs: 0x24,
	SPIMISOOffs: 0x98,
	SPIW0Offs:   0x58,

	SPIMISODLenOffs: 0x28,
	SPIMOSIDLenOffs: 0x24,

	SPIAddrRegMSB: false,

	UARTDateReg: 0x500CA08C,
	UARTClkDiv:  0x500CA014,
	XTALClkDiv:  1,

	BootloaderFlashOffset: 0x2000,

	SupportsEncryptedFlash: true,
	ROMHasCompressedFlash:  true,
	ROMHasChangeBaud:       true,

	FlashFrequency: map[string]byte{
		"80m": 0xF,
		"40m": 0x0,
		"20m": 0x2,
	},

	FlashSizes: defaultFlashSizes(),

	PostConnect: esp32p4PostConnect,
}

// chipRevision reads the P4 chip revision from EFUSE and returns it as
// major*100 + minor (e.g. v3.1 -> 301), matching esptool. It returns 0 if the
// register cannot be read.
func chipRevision(f *Flasher) int {
	word, err := f.ReadRegister(esp32p4EFUSE_REV_WORD)
	if err != nil {
		return 0
	}
	major := int(((word>>23)&1)<<2) | int((word>>4)&0x03)
	minor := int(word & 0x0F)
	return major*100 + minor
}

// esp32p4PostConnect is revision-aware. It selects the UARTDEV_BUF_NO address
// by silicon revision, detects the USB-JTAG/Serial interface (disabling the LP
// watchdogs so the chip is not reset mid-flash), and powers on the SPI flash
// on rev >= 3.0 silicon, where the flash is powered off by default and must be
// powered before SPI attach.
//
// Reference: esptool/targets/esp32p4.py _post_connect() and power_on_flash().
func esp32p4PostConnect(f *Flasher) error {
	rev := chipRevision(f)
	f.revision = rev
	f.logf("ESP32-P4 chip revision: %d (major=%d)", rev, rev/100)

	// UARTDEV_BUF_NO address is revision-dependent.
	uartDevBufNo := esp32p4UARTDevBufNoRev1
	if rev >= 300 {
		uartDevBufNo = esp32p4UARTDevBufNoRev3
	}

	uartDev, err := f.ReadRegister(uartDevBufNo)
	if err != nil {
		// In secure download mode, the register may be unreadable.
		// Default to non-USB behavior (safe fallback).
		return nil
	}

	if uartDev == esp32p4UARTDevBufNoUSBJTAGSerial {
		f.usesUSB = true
		f.logf("USB-JTAG/Serial interface detected (ESP32-P4 rev %d), disabling watchdogs", rev)
		if err := disableWatchdogsLP(f, esp32p4LPWDTConfig0, esp32p4LPWDTWProtect,
			esp32p4LPWDTSWDConf, esp32p4LPWDTSWDWProtect); err != nil {
			return err
		}
	}

	// The flash is powered off by default on rev 3.0 silicon (ECO6/ECO7).
	// Power it on before SPI attach, otherwise attach_flash() times out.
	// Reference: esptool/targets/esp32p4.py power_on_flash().
	if rev == 301 || rev == 302 {
		f.logf("ESP32-P4 rev %d: powering on SPI flash before attach", rev)
		if err := esp32p4PowerOnFlash(f, rev); err != nil {
			f.logf("Warning: power on flash: %v", err)
		} else {
			f.logf("ESP32-P4 rev %d: SPI flash powered on", rev)
		}
	}

	return nil
}

// esp32p4PowerOnFlash powers on the SPI flash on rev 3.1/3.2 silicon.
// Reference: esptool/targets/esp32p4.py power_on_flash().
func esp32p4PowerOnFlash(f *Flasher, rev int) error {
	// On ECO7 (rev 302), skip the power-up when DOWNLOAD_MODE_XPD_ON is
	// programmed: the ROM already asserts flash XPD in download mode. In that
	// case, clear any lingering "flash force on" bits in PMU_DATE_REG before
	// proceeding, since that path is not safe to run twice while flash is
	// already powered.
	if rev == 302 {
		val, err := f.ReadRegister(esp32p4EFUSE_RD_REPEAT_DATA1)
		if err != nil {
			return err
		}
		if val&esp32p4EFUSEDownloadModeXPdon != 0 {
			date, err := f.ReadRegister(esp32p4PMUDate)
			if err != nil {
				return err
			}
			if date&esp32p4PMUDateFlashForceOn == esp32p4PMUDateFlashForceOn {
				return f.WriteRegister(esp32p4PMUDate, date&^esp32p4PMUDateFlashForceOn)
			}
			return nil
		}
	}

	// Power up the analog pad group.
	if err := f.WriteRegister(esp32p4LPSystemAnaXpdPadGroup, 1); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)

	// Flash power-up sequence. Set the current-limit and force-tiehi bits, then
	// assert flash "force on" in PMU_DATE_REG.
	cur, err := f.ReadRegister(esp32p4PMUExtLDOP0_0P1aAna)
	if err != nil {
		return err
	}
	if err := f.WriteRegister(esp32p4PMUExtLDOP0_0P1aAna, cur|esp32p4PMUAna0P1aEnCurLim); err != nil {
		return err
	}
	cur, err = f.ReadRegister(esp32p4PMUExtLDOP0_0P1a)
	if err != nil {
		return err
	}
	if err := f.WriteRegister(esp32p4PMUExtLDOP0_0P1a, cur|esp32p4PMU0P1aForceTieHSel); err != nil {
		return err
	}
	date, err := f.ReadRegister(esp32p4PMUDate)
	if err != nil {
		return err
	}
	if err := f.WriteRegister(esp32p4PMUDate, date|esp32p4PMUDateFlashForceOn); err != nil {
		return err
	}
	time.Sleep(50 * time.Microsecond)

	// Release the current-limit bit, then pulse the force-tiehi bit on the
	// P0_0P1A register (set 0x80 then clear) and drop the target-voltage and
	// force-tiehi bits. Each step re-reads the register fresh, matching
	// esptool/targets/esp32p4.py so we never clobber neighbouring bits.
	cur, err = f.ReadRegister(esp32p4PMUExtLDOP0_0P1aAna)
	if err != nil {
		return err
	}
	if err := f.WriteRegister(esp32p4PMUExtLDOP0_0P1aAna, cur&^esp32p4PMUAna0P1aEnCurLim); err != nil {
		return err
	}
	cur, err = f.ReadRegister(esp32p4PMUExtLDOP0_0P1a)
	if err != nil {
		return err
	}
	if err := f.WriteRegister(esp32p4PMUExtLDOP0_0P1a, cur&^esp32p4PMU0P1aTarget0); err != nil {
		return err
	}
	cur, err = f.ReadRegister(esp32p4PMUExtLDOP0_0P1a)
	if err != nil {
		return err
	}
	if err := f.WriteRegister(esp32p4PMUExtLDOP0_0P1a, cur|0x80); err != nil {
		return err
	}
	cur, err = f.ReadRegister(esp32p4PMUExtLDOP0_0P1a)
	if err != nil {
		return err
	}
	if err := f.WriteRegister(esp32p4PMUExtLDOP0_0P1a, cur&^esp32p4PMU0P1aForceTieHSel); err != nil {
		return err
	}
	time.Sleep(1800 * time.Microsecond)

	return nil
}
