package flash

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/georgik/espbrew-go/internal/chips"
	"github.com/georgik/espbrew-go/internal/flash/espfmt"
	"github.com/rs/zerolog/log"
)

// ConvertELFToESPImage converts an ELF file to ESP-IDF binary format with bootloader.
//
// revision selects the revision-aware ESP32-P4 bootloader variant (v0 for
// rev < 3.0, v3 for rev >= 3.0); it is ignored for every other chip. A revision
// of 0 means "unknown" and falls back to the v0 variant.
func ConvertELFToESPImage(elfData []byte, chip chips.Chip, revision int) ([]byte, error) {
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

	// Create image builder
	builder := espfmt.NewImageBuilder(espfmt.Chip(chip))
	builder.SetEntry(entryPoint)

	// Stamp the ELF SHA256 into the app descriptor's app_elf_sha256 field,
	// matching espflash (image_format/idf.rs). The descriptor is a fixed 256-
	// byte region at the start of its segment, so writing the hash IN PLACE at
	// its app_elf_sha256 field (offset 144) never shifts or overwrites code.
	elfSHA256 := sha256.Sum256(elfData)
	builder.SetAppDescriptorSha256(elfSHA256)
	log.Info().Str("sha256", fmt.Sprintf("%x", elfSHA256)).Msg("ELF SHA256 calculated")

	// Set flash size based on chip type. The ESP32-S3-Box and the ESP32-P4
	// Function Eval Board ship with 16MB of flash, so their app image header
	// must declare 16MB (flash_config 0x40) to match espflash; a 4MB declaration
	// would make the header disagree with the board and the bootloader.
	flashSize := 4 * 1024 * 1024
	if chip == chips.ChipESP32S3 || chip == chips.ChipESP32P4 {
		flashSize = 16 * 1024 * 1024
		log.Info().Int("flash_size_mb", 16).Msg("Using 16MB flash size")
	}
	builder.SetFlashSize(espfmt.FlashSizeFromMB(uint32(flashSize / (1024 * 1024))))

	// Get bootloader (downloads from GitHub if not cached)
	bootloader, err := espfmt.GetBootloader(espfmt.Chip(chip), espfmt.Xtal40MHz, revision)
	if err != nil {
		log.Warn().Err(err).Msg("No bootloader available, building app-only image")
	} else {
		// Patch the bootloader header's flash-config byte so the bootloader
		// reads the SPI flash at 40 MHz. The ROM and 2nd-stage bootloader read
		// flash using the flash-config byte in the *bootloader's own* header,
		// not the app header. The bundled P4 bootloaders ship with an 80 MHz
		// config; flashing them verbatim makes the 2nd-stage bootloader read
		// the app at 80 MHz and crash with an Illegal instruction on flash that
		// cannot keep up. espflash patches this byte (image_format/idf.rs)
		// before flashing, defaulting to the chip's default frequency (40 MHz
		// for P4). We replicate that here so the image boots the same way.
		//
		// espflash uses the default flash size (4 MB) and the default
		// frequency; the frequency nibble (0x0 = 40 MHz) is what sets the read
		// speed, giving flash-config byte 0x20.
		bootloader = espfmt.PatchBootloaderFlashConfig(
			bootloader,
			espfmt.FlashSize4MB,
			espfmt.FlashFreq40MHz,
		)
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
