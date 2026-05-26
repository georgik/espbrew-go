package flash

import (
	"encoding/binary"
	"errors"
	"fmt"

	"codeberg.org/georgik/espbrew-go/internal/chips"
)

var (
	// ErrInvalidELF is returned when the file is not a valid ELF
	ErrInvalidELF = errors.New("invalid ELF file")
	// ErrNoSegments is returned when no loadable segments are found
	ErrNoSegments = errors.New("no loadable segments found")
)

// Flash address ranges for different chips
type flashRange struct {
	IROMStart, IROMEnd uint32
	DROMStart, DROMEnd uint32
	IRAMStart, IRAMEnd uint32 // IRAM range for startup code
}

var flashRanges = map[chips.Chip]flashRange{
	chips.ChipESP32: {
		IROMStart: 0x400D0000, IROMEnd: 0x40400000,
		DROMStart: 0x3F400000, DROMEnd: 0x3F800000,
		IRAMStart: 0x40080000, IRAMEnd: 0x400A0000,
	},
	chips.ChipESP32S2: {
		IROMStart: 0x40080000, IROMEnd: 0x41800000,
		DROMStart: 0x3F000000, DROMEnd: 0x3F3F0000,
		IRAMStart: 0x40020000, IRAMEnd: 0x40040000,
	},
	chips.ChipESP32S3: {
		IROMStart: 0x42000000, IROMEnd: 0x44000000,
		DROMStart: 0x3C000000, DROMEnd: 0x3E000000,
		IRAMStart: 0x40370000, IRAMEnd: 0x403E0000,
	},
	chips.ChipESP32C3: {
		IROMStart: 0x42000000, IROMEnd: 0x42800000,
		DROMStart: 0x3C000000, DROMEnd: 0x3C800000,
		IRAMStart: 0x40380000, IRAMEnd: 0x403A0000,
	},
	chips.ChipESP32C6: {
		IROMStart: 0x42000000, IROMEnd: 0x42800000,
		DROMStart: 0x3C000000, DROMEnd: 0x3C800000,
		IRAMStart: 0x40380000, IRAMEnd: 0x403A0000,
	},
	chips.ChipESP32H2: {
		IROMStart: 0x42000000, IROMEnd: 0x42800000,
		DROMStart: 0x3C000000, DROMEnd: 0x3C800000,
		IRAMStart: 0x40380000, IRAMEnd: 0x403A0000,
	},
}

// ELFSegment represents a segment from an ELF file
type ELFSegment struct {
	Addr  uint32 // Virtual address
	Data  []byte // Segment data
	IsROM bool   // True if this is a flash (ROM) segment
	IsRAM bool   // True if this is a RAM segment
}

// ELFSection represents a section from an ELF file
type ELFSection struct {
	Name   string
	Addr   uint32
	Data   []byte
	Type   uint32
	Flags  uint32
	Size   uint32
	Offset uint32
}

// ParseELFSections extracts all sections from an ESP32 ELF file
func ParseELFSections(data []byte) ([]ELFSection, error) {
	if len(data) < 64 {
		return nil, ErrInvalidELF
	}

	// Check ELF magic
	if data[0] != 0x7F || data[1] != 'E' || data[2] != 'L' || data[3] != 'F' {
		return nil, ErrInvalidELF
	}

	// Check ELF class (32-bit)
	if data[4] != 1 {
		return nil, fmt.Errorf("only 32-bit ELF files supported")
	}

	// Check endianness (little)
	if data[5] != 1 {
		return nil, fmt.Errorf("only little-endian ELF files supported")
	}

	// Get ELF header fields
	eShoff := binary.LittleEndian.Uint32(data[32:36])     // Section header offset
	eShentsize := binary.LittleEndian.Uint16(data[46:48]) // Section header entry size
	eShnum := binary.LittleEndian.Uint16(data[48:50])     // Section header count
	eShstrndx := binary.LittleEndian.Uint16(data[50:52])  // Section header string table index

	// Get section header string table
	var strTabOffset uint32

	if eShstrndx < eShnum {
		shoff := eShoff + uint32(eShstrndx)*uint32(eShentsize)
		if shoff+40 <= uint32(len(data)) {
			strTabOffset = binary.LittleEndian.Uint32(data[shoff+16 : shoff+20])
		}
	}

	var sections []ELFSection

	// Parse section headers
	for i := uint16(0); i < eShnum; i++ {
		shoff := eShoff + uint32(i)*uint32(eShentsize)
		if shoff+40 > uint32(len(data)) {
			break
		}

		// Section header fields
		nameIdx := binary.LittleEndian.Uint32(data[shoff : shoff+4])
		shType := binary.LittleEndian.Uint32(data[shoff+4 : shoff+8])
		shFlags := binary.LittleEndian.Uint32(data[shoff+8 : shoff+12])
		shAddr := binary.LittleEndian.Uint32(data[shoff+12 : shoff+16])
		shOffset := binary.LittleEndian.Uint32(data[shoff+16 : shoff+20])
		shSize := binary.LittleEndian.Uint32(data[shoff+20 : shoff+24])

		if shSize == 0 || shOffset == 0 || shOffset >= uint32(len(data)) {
			continue
		}

		// Get section name
		name := ""
		if strTabOffset > 0 {
			end := strTabOffset + nameIdx
			if end+32 < uint32(len(data)) {
				for j := uint32(0); j < 32; j++ {
					b := data[end+j]
					if b == 0 {
						break
					}
					name += string(b)
				}
			}
		}

		// Extract section data if it has content
		var sectionData []byte
		if shOffset+shSize <= uint32(len(data)) {
			sectionData = data[shOffset : shOffset+shSize]
		}

		sections = append(sections, ELFSection{
			Name:   name,
			Addr:   shAddr,
			Data:   sectionData,
			Type:   shType,
			Flags:  shFlags,
			Size:   shSize,
			Offset: shOffset,
		})
	}

	return sections, nil
}

// GetROMSegments extracts ROM (flash) segments from ELF sections for a given chip
func GetROMSegments(sections []ELFSection, chip chips.Chip) []ELFSegment {
	var segments []ELFSegment
	ranges, ok := flashRanges[chip]
	if !ok {
		return segments
	}

	for _, section := range sections {
		if len(section.Data) == 0 {
			continue
		}

		// Check if this is a PROGBITS (1) or INIT_ARRAY (14) section
		if section.Type != 1 && section.Type != 14 {
			continue
		}

		// Check if address is in flash range
		isFlash := isAddrInFlash(section.Addr, ranges)

		if isFlash {
			segments = append(segments, ELFSegment{
				Addr:  section.Addr,
				Data:  section.Data,
				IsROM: true,
				IsRAM: false,
			})
		}
	}

	return segments
}

// GetRAMSegments extracts RAM segments from ELF sections for a given chip
func GetRAMSegments(sections []ELFSection, chip chips.Chip) []ELFSegment {
	var segments []ELFSegment
	ranges, ok := flashRanges[chip]
	if !ok {
		return segments
	}

	for _, section := range sections {
		if len(section.Data) == 0 {
			continue
		}

		// Check if this is a PROGBITS (1) or INIT_ARRAY (14) section
		if section.Type != 1 && section.Type != 14 {
			continue
		}

		// Check if address is NOT in flash range
		isFlash := isAddrInFlash(section.Addr, ranges)

		if !isFlash && section.Addr > 0 {
			segments = append(segments, ELFSegment{
				Addr:  section.Addr,
				Data:  section.Data,
				IsROM: false,
				IsRAM: true,
			})
		}
	}

	return segments
}

// isAddrInFlash checks if an address falls within flash ranges (IROM, DROM only)
// IRAM is NOT flash - it's RAM and should be treated separately
func isAddrInFlash(addr uint32, ranges flashRange) bool {
	return (addr >= ranges.IROMStart && addr < ranges.IROMEnd) ||
		(addr >= ranges.DROMStart && addr < ranges.DROMEnd)
}

// MergeSegments merges adjacent or overlapping segments
func MergeSegments(segments []ELFSegment) []ELFSegment {
	if len(segments) == 0 {
		return segments
	}

	// Sort by address
	sorted := make([]ELFSegment, len(segments))
	copy(sorted, segments)

	// Simple bubble sort by address
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Addr > sorted[j].Addr {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var merged []ELFSegment
	current := sorted[0]

	for _, next := range sorted[1:] {
		end := current.Addr + uint32(len(current.Data))
		if next.Addr <= end {
			// Overlapping or adjacent - merge
			newEnd := next.Addr + uint32(len(next.Data))
			if newEnd > end {
				// Add padding up to next segment start
				padding := make([]byte, next.Addr-end)
				current.Data = append(current.Data, padding...)
				// Append next segment data
				current.Data = append(current.Data, next.Data...)
			}
		} else {
			// Non-overlapping - save current and start new
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

	return merged
}

// ParseELF extracts loadable segments from an ESP32 ELF file
// Returns ROM and RAM segments separately for proper ESP-IDF image generation
func ParseELF(data []byte, chip chips.Chip) ([]ELFSegment, []ELFSegment, error) {
	sections, err := ParseELFSections(data)
	if err != nil {
		return nil, nil, err
	}

	romSegments := GetROMSegments(sections, chip)
	ramSegments := GetRAMSegments(sections, chip)

	romSegments = MergeSegments(romSegments)
	// RAM segments are NOT merged - espflash keeps them separate

	if len(romSegments) == 0 && len(ramSegments) == 0 {
		return nil, nil, ErrNoSegments
	}

	return romSegments, ramSegments, nil
}
