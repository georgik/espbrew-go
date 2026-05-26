package flash

import (
	"encoding/binary"
	"fmt"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/flash/espfmt"
	"github.com/rs/zerolog/log"
)

// ConvertELFToESPImage converts an ELF file to ESP-IDF binary format with bootloader
func ConvertELFToESPImage(elfData []byte, chip chips.Chip) ([]byte, error) {
	// Extract entry point from ELF header (offset 24, 4 bytes, little-endian)
	if len(elfData) < 28 {
		return nil, fmt.Errorf("ELF file too short")
	}
	entryPoint := binary.LittleEndian.Uint32(elfData[24:28])
	log.Info().Uint32("entry_point", entryPoint).Msg("ELF entry point")

	// Parse ELF into ROM and RAM segments
	romSegments, ramSegments, err := ParseELF(elfData, chip)
	if err != nil {
		return nil, err
	}

	log.Info().Int("rom_segments", len(romSegments)).Int("ram_segments", len(ramSegments)).Msg("ELF segments extracted")

	// NOTE: We deliberately do NOT populate ELF SHA256 in app descriptor.
	// espflash leaves this as "0.0.0" version string, and populating SHA256
	// was causing boot failures. The SHA256 was overwriting actual code data.
	// elfSHA256 := sha256.Sum256(elfData)
	// log.Info().Str("sha256", fmt.Sprintf("%x", elfSHA256)).Msg("ELF SHA256 calculated")

	// Create image builder
	builder := espfmt.NewImageBuilder(espfmt.Chip(chip))
	builder.SetEntry(entryPoint)

	// Set flash size based on chip type (ESP32-S3 boards typically have 16MB)
	flashSize := 4 * 1024 * 1024
	if chip == chips.ChipESP32S3 {
		flashSize = 16 * 1024 * 1024 // ESP32-S3-Box has 16MB
		log.Info().Int("flash_size_mb", 16).Msg("Using 16MB flash size for ESP32-S3")
	}
	builder.SetFlashSize(espfmt.FlashSizeFromMB(uint32(flashSize / (1024 * 1024))))

	// Get bootloader (downloads from GitHub if not cached)
	bootloader, err := espfmt.GetBootloader(espfmt.Chip(chip), espfmt.Xtal40MHz)
	if err != nil {
		log.Warn().Err(err).Msg("No bootloader available, building app-only image")
	} else {
		builder.SetBootloader(bootloader)
		log.Info().Int("bootloader_size", len(bootloader)).Msg("Using bootloader")
	}

	// Generate default partition table
	partitionTable := espfmt.DefaultPartitionTable(espfmt.Chip(chip), uint32(flashSize))
	builder.SetPartitionTable(partitionTable)

	// Convert ROM segments
	for _, seg := range romSegments {
		builder.AddSegment(seg.Addr, seg.Data)
	}

	// Convert RAM segments (these should be in the image too!)
	for _, seg := range ramSegments {
		builder.AddSegment(seg.Addr, seg.Data)
	}

	// RAM segments can also be used for padding if needed
	var ramSegs []espfmt.Segment
	for _, seg := range ramSegments {
		ramSegs = append(ramSegs, espfmt.Segment{Addr: seg.Addr, Data: seg.Data})
	}
	builder.SetRAMSegments(ramSegs)

	// Build the full image (bootloader + partition + app)
	parts, err := builder.BuildFullImage()
	if err != nil {
		return nil, err
	}

	log.Info().Int("parts", len(parts)).Msg("Image parts built")

	// For ELF files, we need to return bootloader + partition + app
	// Combine all parts into a single format that can be split by the flasher
	// Format: each part prefixed with 8-byte header [offset(4) + length(4)]
	var result []byte
	for _, part := range parts {
		log.Info().Str("part", part.Name).Int("size", len(part.Data)).Uint32("offset", part.Offset).Msg("Including image part")

		// Write part header: offset (4 bytes) + length (4 bytes)
		header := make([]byte, 8)
		header[0] = byte(part.Offset >> 24)
		header[1] = byte(part.Offset >> 16)
		header[2] = byte(part.Offset >> 8)
		header[3] = byte(part.Offset)
		header[4] = byte(len(part.Data) >> 24)
		header[5] = byte(len(part.Data) >> 16)
		header[6] = byte(len(part.Data) >> 8)
		header[7] = byte(len(part.Data))

		result = append(result, header...)
		result = append(result, part.Data...)
	}

	return result, nil
}

// ImagePart represents a single part of the image with offset and data
type ImagePart struct {
	Offset uint32
	Data   []byte
}

// ParseMultiPartImage parses the multi-part image format returned by ConvertELFToESPImage
// Format: [offset(4) + length(4) + data] repeated for each part
func ParseMultiPartImage(data []byte) ([]ImagePart, error) {
	var parts []ImagePart
	offset := 0

	for offset < len(data) {
		if offset+8 > len(data) {
			return nil, fmt.Errorf("invalid multipart image: incomplete header at offset %d", offset)
		}

		// Read offset and length from header
		partOffset := uint32(data[offset])<<24 | uint32(data[offset+1])<<16 |
			uint32(data[offset+2])<<8 | uint32(data[offset+3])
		partLength := uint32(data[offset+4])<<24 | uint32(data[offset+5])<<16 |
			uint32(data[offset+6])<<8 | uint32(data[offset+7])

		offset += 8

		if offset+int(partLength) > len(data) {
			return nil, fmt.Errorf("invalid multipart image: incomplete data at offset %d", offset)
		}

		parts = append(parts, ImagePart{
			Offset: partOffset,
			Data:   data[offset : offset+int(partLength)],
		})

		offset += int(partLength)
	}

	return parts, nil
}
