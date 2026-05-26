package espfmt

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"

	"codeberg.org/georgik/espbrew-go/internal/chips"
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

// isInIROM checks if an address is in the IROM region (needs memory mapping alignment)
func isInIROM(addr uint32) bool {
	return addr >= IROM_START && addr < IROM_END
}

// isInDROM checks if an address is in the DROM region (needs memory mapping alignment)
func isInDROM(addr uint32) bool {
	return addr >= DROM_START && addr < DROM_END
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
	chip        chips.Chip
	flashMode   uint8
	flashSize   uint8
	flashFreq   uint8
	entry       uint32
	segments    []Segment
	romSegments []Segment
	ramSegments []Segment
	mmuPageSize uint32
	bootloader  []byte
	partition   []byte
	lastSegAddr uint32 // Track last segment address for region change detection
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

// AddSegment adds a code/data segment
func (b *ImageBuilder) AddSegment(addr uint32, data []byte) {
	b.segments = append(b.segments, Segment{Addr: addr, Data: data})
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
		if isInIROM(seg.Addr) || isInDROM(seg.Addr) {
			romSegments = append(romSegments, seg)
		} else {
			ramSegments = append(ramSegments, seg)
		}
	}

	// Merge ROM segments (but NOT RAM segments - espflash keeps them separate)
	romMerged := b.mergeSegments(romSegments)

	// Pad ROM segments to 4-byte alignment
	romPadded := b.padSegments(romMerged)

	// Sort RAM segments by address
	sort.Slice(ramSegments, func(i, j int) bool {
		return ramSegments[i].Addr < ramSegments[j].Addr
	})

	// RAM segments are merged only if EXACTLY adjacent (like espflash)
	// Unlike ROM segments, we don't add padding between RAM segments
	ramMerged := b.mergeAdjacentExact(ramSegments)

	// Build image
	var buf bytes.Buffer

	// Build extended header
	header := ExtendedImageHeader{
		Magic:          ESP_MAGIC,
		SegmentCount:   0, // Will be updated later
		FlashMode:      b.flashMode,
		FlashSizeFreq:  (b.flashSize << 4) | b.flashFreq,
		Entry:          b.entry,
		WPPin:          WP_PIN_DISABLED,
		ChipID:         b.chip.ESPChipID(),
		MinRev:         0,
		MinChipRevFull: 0,
		MaxChipRevFull: 0x63, // Support all chip revisions (99 = 0x63)
		AppendDigest:   1,
	}

	// Write header (24 bytes extended header)
	binary.Write(&buf, binary.LittleEndian, header)

	// Build segments with proper alignment
	var checksum uint8 = ESP_CHECKSUM_MAGIC
	segmentCount := 0

	// Separate DROM and IROM segments (they have different alignment requirements)
	var dromSegments []Segment
	var iromSegments []Segment
	for _, seg := range romPadded {
		if isInDROM(seg.Addr) {
			dromSegments = append(dromSegments, seg)
		} else {
			iromSegments = append(iromSegments, seg)
		}
	}

	// Use a mutable copy of ramMerged for padding (espflash uses RAM to fill gaps)
	remainingRam := make([]Segment, len(ramMerged))
	copy(remainingRam, ramMerged)

	// Process DROM segments first (no alignment needed, just write them)
	for _, seg := range dromSegments {
		checksum = b.saveFlashSegment(&buf, seg, checksum, b.mmuPageSize)
		segmentCount++
	}

	// Fill gap with RAM segments before IROM
	// espflash writes RAM segments here to avoid large padding segments
	if len(iromSegments) > 0 {
		// Calculate where IROM needs to be (absolute file offset)
		// IROM alignment: file_offset % IROM_ALIGN == (seg.Addr - SEG_HEADER_LEN) % IROM_ALIGN
		iromSeg := iromSegments[0]
		alignmentOffset := (iromSeg.Addr - SEG_HEADER_LEN) % b.mmuPageSize
		currentPage := buf.Len() / int(b.mmuPageSize)
		targetOffset := currentPage*int(b.mmuPageSize) + int(alignmentOffset)

		// If targetOffset is behind us, move to next page
		if targetOffset < buf.Len() {
			targetOffset = (currentPage+1)*int(b.mmuPageSize) + int(alignmentOffset)
		}

		// Write RAM segments (splitting if needed) until we reach target offset
		for len(remainingRam) > 0 && buf.Len()+8 < targetOffset {
			ramSeg := remainingRam[0]
			remaining := targetOffset - (buf.Len() + 8 + len(ramSeg.Data))

			if remaining >= 0 {
				// Full segment fits
				checksum = b.saveSegment(&buf, ramSeg, checksum)
				segmentCount++
				remainingRam = remainingRam[1:]
			} else {
				// Need to split segment
				splitLen := uint32(targetOffset - (buf.Len() + 8))
				padSeg := Segment{
					Addr: ramSeg.Addr,
					Data: ramSeg.Data[:splitLen],
				}
				checksum = b.saveSegment(&buf, padSeg, checksum)
				segmentCount++
				// Keep remainder for later
				remainingRam[0] = Segment{
					Addr: ramSeg.Addr + splitLen,
					Data: ramSeg.Data[splitLen:],
				}
				break
			}
		}

		// Add padding to align IROM (if any gap remains)
		for buf.Len()+8 < targetOffset {
			padLen := uint32(targetOffset - (buf.Len() + 8))
			padSegHeader := SegmentHeader{Addr: 0, Length: padLen}
			binary.Write(&buf, binary.LittleEndian, padSegHeader)
			for i := uint32(0); i < padLen; i++ {
				buf.WriteByte(0)
				checksum ^= byte(i & 0xFF)
			}
			segmentCount++
		}

		// Write IROM segments
		for _, seg := range iromSegments {
			checksum = b.saveFlashSegment(&buf, seg, checksum, b.mmuPageSize)
			segmentCount++
		}
	}

	// Write remaining RAM segments WITHOUT padding (espflash behavior)
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
	binary.Write(buf, binary.LittleEndian, segHeader)
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
	default:
		// ESP32-S3, C3, C6, H2 use 0x0
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

	// Sort by address
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].Addr < segments[j].Addr
	})

	var merged []Segment
	current := segments[0]

	for _, next := range segments[1:] {
		end := current.Addr + uint32(len(current.Data))
		if next.Addr <= end {
			// Overlapping or adjacent - merge
			newEnd := next.Addr + uint32(len(next.Data))
			if newEnd > end {
				// Extend current segment
				padding := make([]byte, newEnd-end)
				current.Data = append(current.Data, padding...)
			}
		} else {
			// Check if segments can be merged with 4-byte padding
			maxPadding := (4 - end%4) % 4
			if end+maxPadding >= next.Addr {
				// Merge by adding padding
				padding := make([]byte, next.Addr-end)
				current.Data = append(current.Data, padding...)
			} else {
				// Non-overlapping - save current and start new
				merged = append(merged, current)
				current = next
			}
		}
	}
	merged = append(merged, current)

	return merged
}

// mergeAdjacentExact merges only exactly adjacent segments (no padding added)
// Used for RAM segments to match espflash behavior
func (b *ImageBuilder) mergeAdjacentExact(segments []Segment) []Segment {
	if len(segments) == 0 {
		return segments
	}

	var merged []Segment
	current := segments[0]

	for _, next := range segments[1:] {
		end := current.Addr + uint32(len(current.Data))
		if next.Addr == end {
			// Exactly adjacent - merge without padding
			current.Data = append(current.Data, next.Data...)
		} else {
			// Not adjacent - save current and start new
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

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

// DefaultPartitionTable creates a default partition table for the chip
func DefaultPartitionTable(chip Chip, flashSize uint32) []byte {
	const (
		NVS_ADDR      = 0x9000
		NVS_SIZE      = 0x6000
		PHY_INIT_ADDR = 0xF000
		PHY_INIT_SIZE = 0x1000
	)

	var appAddr, appSize uint32
	switch chip {
	case ChipESP32S2:
		appAddr = 0x10000
		appSize = 0x100000 // 1MB
	case ChipESP32S3:
		appAddr = 0x10000
		appSize = 0x100000 // 1MB (can be scaled by flash size)
	default:
		appAddr = 0x10000
		appSize = 0x300000 // 3MB for ESP32/C3
	}

	// Limit app size by available flash
	if appAddr+appSize > flashSize {
		appSize = flashSize - appAddr
	}

	// Build partition table
	var buf bytes.Buffer

	// NVS partition
	writePartition(&buf, "nvs", 0x01, 0x02, NVS_ADDR, NVS_SIZE, 0)

	// PHY init data partition
	writePartition(&buf, "phy_init", 0x01, 0x01, PHY_INIT_ADDR, PHY_INIT_SIZE, 0)

	// Factory app partition
	writePartition(&buf, "factory", 0x00, 0x00, appAddr, appSize, 0)

	// Padding to fill 32-byte entries
	for buf.Len()%32 != 0 {
		buf.WriteByte(0xFF)
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
