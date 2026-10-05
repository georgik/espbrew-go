package wokwi

import (
	"fmt"

	"github.com/georgik/espbrew-go/internal/backend/wokwi/api"
	"github.com/georgik/espbrew-go/internal/chips"
	"github.com/georgik/espbrew-go/internal/flash"
	"github.com/rs/zerolog/log"
)

// appFlashOffset is where the application image lives in ESP32 flash. Every
// ESP32 variant places the app at 0x10000 (the bootloader offset differs per
// chip, but the app offset does not).
const appFlashOffset uint32 = 0x10000

// maxFirstOffset bounds the offset of the first flash section when we try to
// interpret a non-ELF blob as an already-assembled multi-part image. Real
// assembled images start with the bootloader (0x0 or 0x1000) or the partition
// table (0x8000); a raw image misinterpreted by ParseMultiPartImage yields a
// garbage offset that is almost always far larger than this.
const maxFirstOffset uint32 = 0x20000

// assembleSections converts the firmware received for a Wokwi device into Wokwi
// flash sections (offset + bytes). The sections are what get uploaded as
// `flash-<offset>.bin` and referenced in `sim:start` (see
// wiki/wokwi-sim-api-analysis.md §3).
//
// It reuses the exact image-assembly mechanism physical and RUST flashing use:
//
//   - ELF:     ConvertELFToESPImage assembles a bootable image (bootloader +
//     partition table + app) and ParseMultiPartImage splits it into sections.
//   - ESP image / multi-part: an already-assembled espbrew-format image is split
//     directly into sections.
//   - Anything else: a raw blob is uploaded at its natural offset (app offset
//     for an ESP image, 0 for a raw binary).
//
// chip selects the flash/RAM layout used when converting an ELF.
func assembleSections(chip chips.Chip, firmware []byte) ([]api.FlashSection, error) {
	if flash.DetectFileType(firmware) == flash.FileTypeELF {
		combined, err := flash.ConvertELFToESPImage(firmware, chip)
		if err != nil {
			return nil, fmt.Errorf("assemble image from ELF: %w", err)
		}
		parts, err := flash.ParseMultiPartImage(combined)
		if err != nil {
			return nil, fmt.Errorf("parse assembled image: %w", err)
		}
		return toSections(parts), nil
	}

	// Non-ELF: try to interpret an already-assembled espbrew-format image as
	// sections (bootloader + partition + app); fall back to a single blob
	// otherwise. ParseMultiPartImage only succeeds for a well-formed image, and
	// multiPartValid rejects the garbage offset a raw image would produce.
	if parts, err := flash.ParseMultiPartImage(firmware); err == nil && multiPartValid(parts) {
		return toSections(parts), nil
	}

	// Single blob. App images live at the app offset; raw blobs at 0.
	offset := uint32(0)
	if flash.DetectFileType(firmware) == flash.FileTypeESP32Binary {
		offset = appFlashOffset
	}
	return []api.FlashSection{{Offset: int(offset), Data: firmware}}, nil
}

// toSections converts flash image parts into Wokwi API flash sections.
func toSections(parts []flash.ImagePart) []api.FlashSection {
	sections := make([]api.FlashSection, 0, len(parts))
	for _, p := range parts {
		sections = append(sections, api.FlashSection{Offset: int(p.Offset), Data: p.Data})
	}
	return sections
}

// multiPartValid reports whether parts look like a genuine assembled image
// rather than a single image misinterpreted by ParseMultiPartImage. It requires
// at least one part whose offset sits in the bootloader/partition/app region.
func multiPartValid(parts []flash.ImagePart) bool {
	if len(parts) == 0 {
		return false
	}
	return parts[0].Offset >= 0 && parts[0].Offset <= maxFirstOffset
}

// chipForImage resolves the chip used to assemble the flash image from the
// device config, falling back to ESP32-S3 (the espbrew default) when the config
// does not specify one.
func (m *APIMonitor) chipForImage() chips.Chip {
	if c, ok := chips.ParseChip(m.config.ChipType); ok {
		return c
	}
	log.Warn().Str("chip_type", m.config.ChipType).Msg("Unknown chip type, defaulting to ESP32-S3")
	return chips.ChipESP32S3
}

// defaultFlashSizeBytes returns the flash-size hint passed to sim:start. The
// assembled image already carries its flash size in its own header; this is
// only an informational hint for the simulator, so a 4MB default is used.
func defaultFlashSizeBytes() *int {
	size := 4 * 1024 * 1024
	return &size
}
