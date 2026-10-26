package espfmt

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/georgik/espbrew-go/internal/chips"
	"github.com/rs/zerolog/log"
)

const (
	// ESP magic byte
	ESP_MAGIC = 0xE9
	// ESP checksum magic
	ESP_CHECKSUM_MAGIC = 0xEF
	// IROM alignment for ESP32-S3 and later
	IROM_ALIGN = 0x10000
	// Segment header length
	SEG_HEADER_LEN = 8
	// WP pin disabled value
	WP_PIN_DISABLED = 0xEE
	// Flash base address for app image
	FLASH_BASE = 0x10000

	// Partition table constants (matching ESP-IDF esp_flash_partitions.h).
	//
	// The ESP32 bootloader reads the partition table as a fixed 3072-byte
	// region at offset 0x8000. It is a sequence of 32-byte entries, each
	// prefixed with the 0x50AA magic. The real partitions are followed by a
	// single 0xEBEB marker entry whose 16-byte MD5 checksum (computed over
	// every preceding byte) is stored at entry+16; the remainder is padded
	// with 0xFF, which the bootloader treats as the terminating entry.
	//
	// See: components/bootloader_support/include/esp_flash_partitions.h and
	// components/esp_partition/partition.c (esp_partition_table_verify).
	ESP_PARTITION_TABLE_LEN    = 0xC00 // 3072 bytes, ESP_PARTITION_TABLE_MAX_LEN
	ESP_PARTITION_ENTRY_SIZE   = 32
	ESP_PARTITION_MAGIC        = 0x50AA
	ESP_PARTITION_MAGIC_MD5    = 0xEBEB
	ESP_PARTITION_MD5_OFFSET   = 16
	ESP_PARTITION_PADDING_BYTE = 0xFF
)

// Memory region definitions for ESP32-S3
const (
	// DROM region: 0x3C000000 - 0x3E000000
	DROM_START = 0x3C000000
	DROM_END   = 0x3E000000
	// IROM region: 0x42000000 - 0x42800000
	IROM_START = 0x42000000
	IROM_END   = 0x42800000
	// IRAM region: 0x40370000 - 0x403E0000
	IRAM_START = 0x40370000
	IRAM_END   = 0x403E0000
	// DRAM region: 0x3FC88000 - 0x3FD00000
	DRAM_START = 0x3FC88000
	DRAM_END   = 0x3FD00000
)

// sameMemoryRegion checks if two addresses are in the same memory region
// Different regions don't need padding between them
func sameMemoryRegion(addr1, addr2 uint32) bool {
	inDROM := func(addr uint32) bool { return addr >= DROM_START && addr < DROM_END }
	inIROM := func(addr uint32) bool { return addr >= IROM_START && addr < IROM_END }
	inIRAM := func(addr uint32) bool { return addr >= IRAM_START && addr < IRAM_END }
	inDRAM := func(addr uint32) bool { return addr >= DRAM_START && addr < DRAM_END }

	return (inDROM(addr1) && inDROM(addr2)) ||
		(inIROM(addr1) && inIROM(addr2)) ||
		(inIRAM(addr1) && inIRAM(addr2)) ||
		(inDRAM(addr1) && inDRAM(addr2))
}

// chipFlashRegion returns the IROM and DROM (flash) address windows for a chip.
// The bootloader classifies ELF segments as flash or RAM using these windows,
// exactly as espflash does via Chip::addr_is_flash / the chip metadata memory
// layout. The windows differ per chip: most chips map flash at 0x42000000, but
// the ESP32-P4 maps its flash at 0x40000000.
func chipFlashRegion(chip Chip) (iromStart, iromEnd, dromStart, dromEnd uint32) {
	switch chip {
	case ChipESP32P4:
		// ESP32-P4: flash (SOC_IROM/SOC_DROM) mapped at 0x40000000.
		return 0x40000000, 0x42000000, 0x40000000, 0x42000000
	default:
		// ESP32-S3, C3, C6, H2, ...: flash mapped at 0x42000000.
		return IROM_START, IROM_END, DROM_START, DROM_END
	}
}

// isInIROM checks if an address is in the chip's IROM region (needs memory mapping alignment)
func isInIROM(chip Chip, addr uint32) bool {
	s, e, _, _ := chipFlashRegion(chip)
	return addr >= s && addr < e
}

// isInDROM checks if an address is in the chip's DROM region (needs memory mapping alignment)
func isInDROM(chip Chip, addr uint32) bool {
	_, _, s, e := chipFlashRegion(chip)
	return addr >= s && addr < e
}

// ExtendedImageHeader is the extended ESP image header (24 bytes)
type ExtendedImageHeader struct {
	Magic          uint8
	SegmentCount   uint8
	FlashMode      uint8
	FlashSizeFreq  uint8
	Entry          uint32
	WPPin          uint8
	ClkQDrv        uint8
	D_CSDrv        uint8
	GD_WPDrv       uint8
	ChipID         uint16
	MinRev         uint8
	MinChipRevFull uint16
	MaxChipRevFull uint16
	Reserved       [4]uint8
	AppendDigest   uint8
}

// SegmentHeader is a segment header in the ESP image
type SegmentHeader struct {
	Addr   uint32
	Length uint32
}

// Segment represents a code/data segment
type Segment struct {
	Addr uint32
	Data []byte
}

// ImageBuilder builds ESP-IDF format images
type ImageBuilder struct {
	chip          chips.Chip
	flashMode     uint8
	flashSize     uint8
	flashFreq     uint8
	entry         uint32
	segments      []Segment
	romSegments   []Segment
	ramSegments   []Segment
	mmuPageSize   uint32
	bootloader    []byte
	partition     []byte
	appDescSha256 *[32]byte // ELF SHA256 to stamp into the app descriptor
	lastSegAddr   uint32    // Track last segment address for region change detection
}

// Chip is an alias for chips.Chip for convenience
type Chip = chips.Chip

// Chip constants for backward compatibility
const (
	ChipESP32    = chips.ChipESP32
	ChipESP32S2  = chips.ChipESP32S2
	ChipESP32S3  = chips.ChipESP32S3
	ChipESP32C3  = chips.ChipESP32C3
	ChipESP32C6  = chips.ChipESP32C6
	ChipESP32H2  = chips.ChipESP32H2
	ChipESP32C2  = chips.ChipESP32C2
	ChipESP32C5  = chips.ChipESP32C5
	ChipESP32C61 = chips.ChipESP32C61
	ChipESP32P4  = chips.ChipESP32P4
	ChipESP32H4  = chips.ChipESP32H4
	ChipESP32S31 = chips.ChipESP32S31
)

// FlashMode represents the flash read mode
type FlashMode uint8

const (
	FlashModeQIO       FlashMode = 0x00
	FlashModeQOUT      FlashMode = 0x01
	FlashModeDIO       FlashMode = 0x02
	FlashModeDOUT      FlashMode = 0x03
	FlashModeFAST_READ FlashMode = 0x04
	FlashModeSLOW_READ FlashMode = 0x05
)

// FlashSize represents the flash size configuration
type FlashSize uint8

const (
	FlashSize1MB   FlashSize = 0x00
	FlashSize2MB   FlashSize = 0x01
	FlashSize4MB   FlashSize = 0x02
	FlashSize8MB   FlashSize = 0x03
	FlashSize16MB  FlashSize = 0x04
	FlashSize32MB  FlashSize = 0x05
	FlashSize64MB  FlashSize = 0x06
	FlashSize128MB FlashSize = 0x07
)

// FlashFreq represents the flash frequency
type FlashFreq uint8

const (
	FlashFreq40MHz FlashFreq = 0x0
	FlashFreq26MHz FlashFreq = 0x1
	FlashFreq20MHz FlashFreq = 0x2
	FlashFreq80MHz FlashFreq = 0xF
)

// FlashSizeFromMB converts megabytes to FlashSize enum
func FlashSizeFromMB(mb uint32) FlashSize {
	switch mb {
	case 1:
		return FlashSize1MB
	case 2:
		return FlashSize2MB
	case 4:
		return FlashSize4MB
	case 8:
		return FlashSize8MB
	case 16:
		return FlashSize16MB
	case 32:
		return FlashSize32MB
	case 64:
		return FlashSize64MB
	case 128:
		return FlashSize128MB
	default:
		log.Warn().Uint32("mb", mb).Msg("Unknown flash size, defaulting to 4MB")
		return FlashSize4MB
	}
}

// NewImageBuilder creates a new image builder
func NewImageBuilder(chip Chip) *ImageBuilder {
	return &ImageBuilder{
		chip:        chip,
		flashMode:   uint8(FlashModeDIO),
		flashSize:   uint8(FlashSize4MB),
		flashFreq:   uint8(FlashFreq40MHz),
		mmuPageSize: IROM_ALIGN,
		lastSegAddr: 0xFFFFFFFF, // Initialize to invalid address
	}
}

// SetBootloader sets the bootloader binary
func (b *ImageBuilder) SetBootloader(data []byte) {
	b.bootloader = data
}

// SetPartitionTable sets the partition table binary
func (b *ImageBuilder) SetPartitionTable(data []byte) {
	b.partition = data
}

// SetEntry sets the entry point address
func (b *ImageBuilder) SetEntry(addr uint32) {
	b.entry = addr
}

// SetFlashMode sets the flash mode
func (b *ImageBuilder) SetFlashMode(mode FlashMode) {
	b.flashMode = uint8(mode)
}

// SetFlashSize sets the flash size
func (b *ImageBuilder) SetFlashSize(size FlashSize) {
	b.flashSize = uint8(size)
}

// SetFlashFreq sets the flash frequency
func (b *ImageBuilder) SetFlashFreq(freq FlashFreq) {
	b.flashFreq = uint8(freq)
}

// SetROMSegments sets ROM (flash) segments
func (b *ImageBuilder) SetROMSegments(segments []Segment) {
	b.romSegments = segments
}

// SetRAMSegments sets RAM segments
func (b *ImageBuilder) SetRAMSegments(segments []Segment) {
	b.ramSegments = segments
}

// SetMMUPageSize sets the MMU page size
func (b *ImageBuilder) SetMMUPageSize(size uint32) {
	b.mmuPageSize = size
}

// SetAppDescriptorSha256 sets the SHA256 of the source ELF file. espflash
// stamps this into the app descriptor's app_elf_sha256 field (offset 144 in
// esp_app_desc_t); we replicate it so the produced image matches espflash.
func (b *ImageBuilder) SetAppDescriptorSha256(sha [32]byte) {
	b.appDescSha256 = &sha
}

// AddSegment adds a code/data segment
func (b *ImageBuilder) AddSegment(addr uint32, data []byte) {
	b.segments = append(b.segments, Segment{Addr: addr, Data: data})
}

// appDescriptorMagicWord is ESP_APP_DESC_MAGIC_WORD. Every valid app descriptor
// starts with it, so we can locate the descriptor inside its segment without
// re-parsing the ELF (which is what espflash does via .flash.appdesc).
const appDescriptorMagicWord uint32 = 0xABCD5432

// hasAppDescriptorMagic reports whether seg.Data contains an app descriptor
// (identified by its magic word) anywhere in its first appDescriptorSize bytes.
func hasAppDescriptorMagic(data []byte) bool {
	// The magic is the first 4 bytes of an app descriptor; the descriptor is
	// always at the start of the segment that contains it.
	if len(data) < 4 {
		return false
	}
	return readU32LE(data[0:4]) == appDescriptorMagicWord
}

// appDescriptorMMUPageSize reads the "MMU page size in log base 2" field from an
// app descriptor and returns the page size (1 << field). It returns 0 when the
// descriptor is absent or the field is 0, signalling the caller to fall back to
// the builder's default page size.
func appDescriptorMMUPageSize(data []byte) uint32 {
	if len(data) < 4+180 {
		return 0
	}
	field := data[4+180-4] // mmu_page_size sits 180 bytes after the magic start
	if field == 0 || field > 16 {
		return 0
	}
	return uint32(1) << field
}

// readU32LE reads a little-endian u32.
func readU32LE(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// segmentPadding calculates padding needed for IROM/DROM alignment
// Matches espflash: align so file_offset % IROM_ALIGN == segment.addr % IROM_ALIGN
func segmentPadding(offset int, segment Segment, alignTo uint32) uint32 {
	alignPast := (segment.Addr - SEG_HEADER_LEN) % alignTo
	padLen := ((alignTo - uint32(offset%int(alignTo))) + alignPast) % alignTo

	if padLen == 0 || padLen%alignTo == 0 {
		return 0
	}
	if padLen > SEG_HEADER_LEN {
		return padLen - SEG_HEADER_LEN
	}
	return padLen + alignTo - SEG_HEADER_LEN
}

// BuildAppImage builds the application image (without bootloader/partition table)
func (b *ImageBuilder) BuildAppImage() ([]byte, error) {
	// Merge adjacent segments
	// Use b.segments if set (for direct usage), otherwise use b.romSegments (for ELF conversion)
	var segmentsToProcess []Segment
	if len(b.segments) > 0 {
		segmentsToProcess = b.segments
	} else {
		segmentsToProcess = b.romSegments
	}

	// Separate ROM and RAM segments
	// ROM: IROM (0x42000000-0x42800000) and DROM (0x3C000000-0x3E000000)
	// RAM: IRAM (0x40370000-0x403E0000) and DRAM (0x3FC88000-0x3FD00000)
	var romSegments []Segment
	var ramSegments []Segment
	for _, seg := range segmentsToProcess {
		if isInIROM(b.chip, seg.Addr) || isInDROM(b.chip, seg.Addr) {
			romSegments = append(romSegments, seg)
		} else {
			ramSegments = append(ramSegments, seg)
		}
	}

	// Merge adjacent segments, then pad to 4-byte alignment.
	//
	// This mirrors espflash exactly: `pad_align_segments(merge_adjacent_segments(..))`
	// is applied to BOTH the flash segments and the RAM segments. Merging first
	// avoids the overlapping-segment problem that padding-then-merging would
	// introduce, and padding last keeps every segment 4-byte aligned so the
	// bootloader can trust the SegmentHeader.Length field.
	//
	// NOTE: unlike the old code, RAM segments are merged with the same
	// 4-byte-gap tolerance as ROM segments (mergeSegments), not only when
	// exactly adjacent. That is what espflash does, and it is required for the
	// gap-filling before flash segments to line up.
	flashSegments := b.padSegments(b.mergeSegments(romSegments))
	ramMerged := b.padSegments(b.mergeSegments(ramSegments))

	// Build image
	var buf bytes.Buffer

	// Build the 24-byte extended image header.
	//
	// We mirror espflash exactly: the app header is derived from the
	// bootloader's header, then specific fields are overridden. Building the
	// header from the bootloader yields the correct max_chip_rev_full (the
	// ESP32-P4 v3 bootloader declares 399), which the P4 v3 bootloader checks
	// against the silicon revision. The previous hardcoded 0xFFFF made the P4
	// v3 bootloader reject the image with
	// "chip revision check failed. Required <= v0.99, found vX.X".
	var header ExtendedImageHeader
	if len(b.bootloader) >= 24 {
		// Copy the 24-byte header from the bootloader verbatim, then override
		// the fields espflash overrides when building the app header (see
		// image_format/idf.rs): flash_config and entry always change; wp_pin,
		// chip_id and append_digest are normalised. The remaining fields
		// (flash_mode, min_rev, min/max chip rev, reserved) are kept from the
		// bootloader.
		header = ExtendedImageHeader{
			Magic:          b.bootloader[0],
			SegmentCount:   b.bootloader[1],
			FlashMode:      b.bootloader[2],
			FlashSizeFreq:  b.bootloader[3],
			Entry:          binary.LittleEndian.Uint32(b.bootloader[4:8]),
			WPPin:          b.bootloader[8],
			ClkQDrv:        b.bootloader[9],
			D_CSDrv:        b.bootloader[10],
			GD_WPDrv:       b.bootloader[11],
			ChipID:         binary.LittleEndian.Uint16(b.bootloader[12:14]),
			MinRev:         b.bootloader[14],
			MinChipRevFull: binary.LittleEndian.Uint16(b.bootloader[15:17]),
			MaxChipRevFull: binary.LittleEndian.Uint16(b.bootloader[17:19]),
			AppendDigest:   b.bootloader[23],
		}
		// Keep the bootloader's reserved bytes (offsets 19..23).
		copy(header.Reserved[:], b.bootloader[19:23])

		// Override the fields espflash overrides.
		header.FlashSizeFreq = (b.flashSize << 4) | b.flashFreq
		header.Entry = b.entry
		header.WPPin = WP_PIN_DISABLED
		header.ChipID = b.chip.ESPChipID()
		header.AppendDigest = 1
	} else {
		// No bootloader available (e.g. chips without an embedded bootloader):
		// keep the previous hand-constructed header so behaviour is unchanged.
		header = ExtendedImageHeader{
			Magic:          ESP_MAGIC,
			SegmentCount:   0, // Will be updated later
			FlashMode:      b.flashMode,
			FlashSizeFreq:  (b.flashSize << 4) | b.flashFreq,
			Entry:          b.entry,
			WPPin:          WP_PIN_DISABLED,
			ChipID:         b.chip.ESPChipID(),
			MinRev:         0,
			MinChipRevFull: 0,
			// Support all chip revisions. espflash uses u16::MAX here; the old
			// 0x63 (= 99 -> v0.99) capped the max supported revision at v0.99,
			// which made the P4 v3 bootloader reject rev >= 3.0 silicon.
			MaxChipRevFull: 0xFFFF,
			AppendDigest:   1,
		}
	}

	// Write header (24 bytes extended header)
	_ = binary.Write(&buf, binary.LittleEndian, header)

	// Build segments with proper alignment
	var checksum uint8 = ESP_CHECKSUM_MAGIC
	segmentCount := 0

	// Place every flash segment with per-segment MMU-page alignment, filling
	// the gaps with RAM-segment data (or zeros) — exactly like espflash.
	//
	// This is the core of the P4 fix. The ESP32 (including P4) bootloader
	// verifies that each flash segment's data lands at a file offset whose
	// value modulo the MMU page size equals the segment's load address modulo
	// that page size. The old code only aligned the first IROM segment and then
	// wrote the rest back-to-back, so every later P4 segment landed at the
	// wrong offset and the v3 bootloader rejected the image with
	// "Segment N load address 0x…, doesn't match data 0x…".
	//
	// We no longer split DROM from IROM: espflash treats every flash segment
	// uniformly, and for the P4 the whole flash window (0x40000000..) is a
	// single region anyway.

	// Bubble the app descriptor segment to the first position (like espflash)
	// and pick up the MMU page size it requests. Bubbling after merging/padding
	// is safe because the descriptor's bytes do not move, only its order.
	appDescIndex := -1
	mmuPageSize := b.mmuPageSize
	for i := range flashSegments {
		if hasAppDescriptorMagic(flashSegments[i].Data) {
			appDescIndex = i
			if ps := appDescriptorMMUPageSize(flashSegments[i].Data); ps > 0 {
				mmuPageSize = ps
			}
			break
		}
	}
	if appDescIndex >= 0 && appDescIndex != 0 {
		flashSegments[0], flashSegments[appDescIndex] = flashSegments[appDescIndex], flashSegments[0]
	}

	// Stamp the ELF SHA256 into the app descriptor's app_elf_sha256 field,
	// matching espflash (image_format/idf.rs). The descriptor sits at the
	// start of its segment, and app_elf_sha256 is at offset 144. This must
	// happen before the segments are written so the image checksum covers it.
	if b.appDescSha256 != nil {
		for i := range flashSegments {
			if hasAppDescriptorMagic(flashSegments[i].Data) {
				const appDescShaOffset = 144
				if appDescShaOffset+32 <= uint32(len(flashSegments[i].Data)) {
					copy(flashSegments[i].Data[appDescShaOffset:appDescShaOffset+32], b.appDescSha256[:])
				}
				break
			}
		}
	}

	remainingRam := ramMerged
	for _, seg := range flashSegments {
	flushLoop:
		for {
			padLen := segmentPadding(buf.Len(), seg, mmuPageSize)

			switch {
			case padLen > SEG_HEADER_LEN && len(remainingRam) > 0:
				// Fill the gap with RAM-segment data. Split up to padLen bytes
				// off the first RAM segment and save them as a filler segment;
				// the remainder is kept for later (like espflash split_off).
				ram := remainingRam[0]
				take := padLen
				if take > uint32(len(ram.Data)) {
					take = uint32(len(ram.Data))
				}
				checksum = b.saveSegment(&buf, Segment{Addr: ram.Addr, Data: ram.Data[:take]}, checksum)
				segmentCount++
				if take < uint32(len(ram.Data)) {
					remainingRam[0] = Segment{Addr: ram.Addr + take, Data: ram.Data[take:]}
				} else {
					remainingRam = remainingRam[1:]
				}
			case padLen > 0:
				// Fill the gap with a zero-filled dummy segment (no 4-byte
				// alignment padding, exactly like espflash).
				var hdr [SEG_HEADER_LEN]byte
				binary.LittleEndian.PutUint32(hdr[0:], 0)
				binary.LittleEndian.PutUint32(hdr[4:], padLen)
				buf.Write(hdr[:])
				buf.Write(make([]byte, padLen))
				segmentCount++
			default:
				// padLen == 0: segment is aligned, stop filling.
				// NOTE: this must be a labeled break — a bare break would
				// only exit the switch, not the loop, and spin forever.
				break flushLoop
			}
		}

		checksum = b.saveFlashSegment(&buf, seg, checksum, mmuPageSize)
		segmentCount++
	}

	// Write any RAM segments that were not consumed by gap filling.
	for _, seg := range remainingRam {
		checksum = b.saveSegment(&buf, seg, checksum)
		segmentCount++
	}

	// Pad to 16-byte boundary before checksum
	padding := 15 - (buf.Len() % 16)
	buf.Write(make([]byte, padding))

	// Write checksum to buffer
	buf.WriteByte(checksum)

	// Patch segment count AFTER checksum is written
	imageData := buf.Bytes()
	if len(imageData) < 24 {
		return nil, fmt.Errorf("buffer too small: %d bytes", len(imageData))
	}
	imageData[1] = byte(segmentCount)

	log.Debug().Int("segment_count", segmentCount).Uint8("checksum", checksum).Msg("Final checksum")

	// Write SHA256 - hash includes everything written so far (including checksum)
	hasher := sha256.New()
	hasher.Write(imageData)
	hash := hasher.Sum(nil)
	imageData = append(imageData, hash...)

	return imageData, nil
}

// saveSegment writes a segment to the buffer
func (b *ImageBuilder) saveSegment(buf *bytes.Buffer, segment Segment, checksum uint8) uint8 {
	padding := (4 - len(segment.Data)%4) % 4

	segHeader := SegmentHeader{
		Addr:   segment.Addr,
		Length: uint32(len(segment.Data)) + uint32(padding),
	}
	_ = binary.Write(buf, binary.LittleEndian, segHeader)
	buf.Write(segment.Data)

	// Write padding
	if padding > 0 {
		buf.Write(make([]byte, padding))
	}

	// Update checksum
	for _, b := range segment.Data {
		checksum ^= b
	}

	for i := 0; i < padding; i++ {
		checksum ^= 0
	}

	return checksum
}

// saveFlashSegment writes a flash segment with special handling for MMU page boundary
func (b *ImageBuilder) saveFlashSegment(buf *bytes.Buffer, segment Segment, checksum uint8, mmuPageSize uint32) uint8 {
	endPos := uint32(buf.Len() + 8 + len(segment.Data))
	segmentRemainder := endPos % mmuPageSize

	// Add padding if near page boundary (workaround for ESP-IDF bootloader bug)
	// Make a copy to avoid modifying the original segment data
	dataToWrite := segment.Data
	if segmentRemainder < 0x24 {
		padding := make([]byte, 0x24-segmentRemainder)
		// Append to the copy, not the original
		dataToWrite = append(dataToWrite, padding...)
	}

	seg := Segment{Addr: segment.Addr, Data: dataToWrite}
	return b.saveSegment(buf, seg, checksum)
}

// BuildFullImage builds complete image with bootloader, partition table, and app
func (b *ImageBuilder) BuildFullImage() ([]ImagePart, error) {
	parts := []ImagePart{}

	// Add bootloader at appropriate offset
	if len(b.bootloader) > 0 {
		offset := b.bootloaderOffset()
		parts = append(parts, ImagePart{
			Name:   "bootloader",
			Offset: offset,
			Data:   b.bootloader,
		})
	}

	// Add partition table at 0x8000
	if len(b.partition) > 0 {
		parts = append(parts, ImagePart{
			Name:   "partition_table",
			Offset: 0x8000,
			Data:   b.partition,
		})
	}

	// Build and add app image
	appData, err := b.BuildAppImage()
	if err != nil {
		return nil, err
	}

	// App offset depends on chip
	appOffset := b.appOffset()
	parts = append(parts, ImagePart{
		Name:   "app",
		Offset: appOffset,
		Data:   appData,
	})

	return parts, nil
}

// bootloaderOffset returns the flash offset for bootloader
func (b *ImageBuilder) bootloaderOffset() uint32 {
	switch b.chip {
	case ChipESP32, ChipESP32S2:
		return 0x1000
	case ChipESP32P4:
		// espflash: Chip::boot_address() returns 0x2000 for the ESP32-P4.
		return 0x2000
	default:
		// ESP32-S3, C3, C6, C61, H2 use 0x0.
		return 0x0
	}
}

// appOffset returns the flash offset for application
func (b *ImageBuilder) appOffset() uint32 {
	return 0x10000
}

// mergeSegments merges adjacent or overlapping segments
// Also merges segments that are within 4 bytes (can be padded)
func (b *ImageBuilder) mergeSegments(segments []Segment) []Segment {
	if len(segments) == 0 {
		return segments
	}

	// Sort by address, mirroring espflash's merge_adjacent_segments.
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].Addr < segments[j].Addr
	})

	// Merge logic mirrors espflash exactly:
	//   * a segment that starts exactly where the current one ends is joined
	//     and its real bytes are appended;
	//   * a segment within 3 bytes is joined too (the bootloader 4-byte
	//     aligns anyway), with the small gap zero-filled;
	//   * otherwise it starts a new merged segment.
	// Crucially, the next segment's REAL bytes are always appended -- never
	// zeros. The old code appended zero padding for the adjacent case, which
	// silently dropped RAM-segment bytes and produced a zero-filled app image
	// that crashed with "Illegal instruction" on the ESP32-P4.
	var merged []Segment
	for _, seg := range segments {
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			lastEnd := last.Addr + uint32(len(last.Data))
			if seg.Addr >= lastEnd {
				if seg.Addr == lastEnd {
					// Contiguous: append the next segment's real bytes.
					last.Data = append(last.Data, seg.Data...)
					continue
				}
				maxPadding := (4 - (lastEnd % 4)) % 4
				if lastEnd+maxPadding >= seg.Addr {
					// Small gap: zero-fill it, then append the real bytes.
					last.Data = append(last.Data, make([]byte, seg.Addr-lastEnd)...)
					last.Data = append(last.Data, seg.Data...)
					continue
				}
			}
		}
		merged = append(merged, seg)
	}

	return merged
}

// padSegments pads segments to 4-byte alignment
func (b *ImageBuilder) padSegments(segments []Segment) []Segment {
	var padded []Segment
	for _, seg := range segments {
		padLen := (4 - (len(seg.Data) % 4)) % 4
		if padLen > 0 {
			newData := make([]byte, 0, len(seg.Data)+padLen)
			newData = append(newData, seg.Data...)
			newData = append(newData, make([]byte, padLen)...)
			seg.Data = newData
		}
		padded = append(padded, seg)
	}
	return padded
}

// ImagePart represents a part of the final image (bootloader, partition, app)
type ImagePart struct {
	Name   string
	Offset uint32
	Data   []byte
}

// DefaultPartitionTable creates a default partition table for the chip.
//
// The output matches the on-flash format expected by the ESP32 bootloader
// (see components/bootloader_support/include/esp_flash_partitions.h and
// components/esp_partition/partition.c): a fixed-size region of
// ESP_PARTITION_TABLE_LEN bytes made up of 32-byte entries. Each real
// partition is prefixed with the 0x50AA magic; the real partitions are
// followed by a single 0xEBEB marker entry whose 16-byte MD5 checksum
// (computed over every preceding byte) is stored at entry+16; the remainder
// of the region is padded with 0xFF, which the bootloader treats as the
// terminating entry.
func DefaultPartitionTable(chip Chip, flashSize uint32) []byte {
	const (
		NVS_ADDR      = 0x9000
		NVS_SIZE      = 0x6000
		PHY_INIT_ADDR = 0xF000
		PHY_INIT_SIZE = 0x1000
	)

	appAddr := uint32(0x10000)

	// maxPartitionSize mirrors espflash's MAX_PARTITION_SIZE constant
	// (src/image_format/idf.rs): a factory partition never exceeds 16 MB.
	const maxPartitionSize = 16 * 1000 * 1024

	// Per-chip base factory size. espflash keeps these only as a fallback for
	// when the flash size is unknown (see default_partition_table); espbrew
	// always knows the flash size, so the base is used only as a last resort.
	var baseSize uint32
	switch chip {
	case ChipESP32S2, ChipESP32S3, ChipESP32S31:
		baseSize = 0x100000 // 1 MB
	default:
		baseSize = 0x3f0000
	}

	// espflash sizes the factory partition to the detected flash size:
	//
	//     min(flash_size - app_addr, MAX_PARTITION_SIZE)
	//
	// (falling back to the per-chip base above only when the size is unknown).
	// espbrew always knows the flash size, so it always applies the formula.
	// On a 16 MB chip `flash - 0x10000` is 0xff0000, but that is capped at
	// MAX_PARTITION_SIZE (0xfa0000) — exactly what espflash produces.
	var appSize uint32
	if flashSize == 0 {
		appSize = baseSize
	} else {
		appSize = flashSize - appAddr
		if appSize > maxPartitionSize {
			appSize = maxPartitionSize
		}
	}

	// Build the real partition entries.
	var buf bytes.Buffer

	// NVS partition
	writePartition(&buf, "nvs", 0x01, 0x02, NVS_ADDR, NVS_SIZE, 0)

	// PHY init data partition
	writePartition(&buf, "phy_init", 0x01, 0x01, PHY_INIT_ADDR, PHY_INIT_SIZE, 0)

	// Factory app partition
	writePartition(&buf, "factory", 0x00, 0x00, appAddr, appSize, 0)

	// Append the MD5 marker entry. The bootloader computes the MD5 over every
	// byte preceding this entry and compares it against the 16-byte digest
	// stored at entry+ESP_PARTITION_MD5_OFFSET.
	markerStart := buf.Len()
	sum := md5.Sum(buf.Bytes())

	// Marker magic (0xEBEB) followed by 0xFF up to the digest offset, then the
	// 16-byte MD5 digest itself.
	var marker [2]byte
	binary.LittleEndian.PutUint16(marker[:], ESP_PARTITION_MAGIC_MD5)
	buf.Write(marker[:])
	digestAbs := markerStart + ESP_PARTITION_MD5_OFFSET
	for buf.Len() < digestAbs {
		buf.WriteByte(ESP_PARTITION_PADDING_BYTE)
	}
	buf.Write(sum[:])

	// Pad the remainder of the fixed-size table region with 0xFF.
	for buf.Len() < ESP_PARTITION_TABLE_LEN {
		buf.WriteByte(ESP_PARTITION_PADDING_BYTE)
	}

	return buf.Bytes()
}

// writePartition writes a single partition entry
func writePartition(buf *bytes.Buffer, label string, pType, subType uint8, offset, size uint32, flags uint32) {
	// Magic
	binary.Write(buf, binary.LittleEndian, uint16(0x50AA))

	// Type
	buf.WriteByte(pType)
	buf.WriteByte(subType)

	// Offset
	binary.Write(buf, binary.LittleEndian, offset)

	// Size
	binary.Write(buf, binary.LittleEndian, size)

	// Label (16 bytes, null-padded)
	labelBytes := []byte(label)
	for len(labelBytes) < 16 {
		labelBytes = append(labelBytes, 0)
	}
	if len(labelBytes) > 16 {
		labelBytes = labelBytes[:16]
	}
	buf.Write(labelBytes)

	// Flags
	binary.Write(buf, binary.LittleEndian, flags)
}

// ComputeChecksum computes the checksum of data
func ComputeChecksum(data []byte) uint8 {
	var checksum uint8 = ESP_CHECKSUM_MAGIC
	for _, b := range data {
		checksum ^= b
	}
	return checksum
}

// ComputeSHA256 computes SHA256 hash (for bootloader hash)
func ComputeSHA256(data []byte) string {
	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}
