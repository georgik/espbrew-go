package bootloaders

import "embed"

// P4Bootloaders holds the ESP32-P4 second-stage bootloaders embedded at build
// time. They are copied verbatim from the espflash repo
// (espflash/resources/bootloaders/) so espbrew can select the right variant
// without a network fetch.
//
// The ESP32-P4 is revision-aware silicon: the bootloader built for
// CONFIG_ESP32P4_REV_MIN_300 (esp32p4-v3) must be used on rev >= 3.0 silicon,
// while esp32p4-v0 is built for rev < 3.0. espflash selects between them by
// chip revision, so we embed both and pick by revision at flash time.
//
//go:embed esp32p4-v0-bootloader.bin esp32p4-v3-bootloader.bin
var P4Bootloaders embed.FS
