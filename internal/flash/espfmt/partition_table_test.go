package espfmt

import (
	"bytes"
	"crypto/md5"
	"strings"
	"testing"
)

// verifyPartitionTableMD5 mirrors the ESP32 bootloader's
// esp_partition_table_verify: it walks 32-byte entries, accumulates an MD5 over
// every entry preceding the 0xEBEB marker, and checks the 16-byte digest stored
// at entry+ESP_PARTITION_MD5_OFFSET. It returns true when the table is valid.
func verifyPartitionTableMD5(table []byte) bool {
	numParts := 0
	for off := 0; off+ESP_PARTITION_ENTRY_SIZE <= len(table); off += ESP_PARTITION_ENTRY_SIZE {
		magic := binaryLittleUint16(table[off:])
		if magic == ESP_PARTITION_MAGIC_MD5 {
			sum := md5.Sum(table[:off])
			digest := table[off+ESP_PARTITION_MD5_OFFSET : off+ESP_PARTITION_MD5_OFFSET+md5.Size]
			return bytes.Equal(digest, sum[:])
		}
		if magic != ESP_PARTITION_MAGIC {
			// 0xFFFF terminator (or anything else) ends the table.
			return true
		}
		numParts++
	}
	return false
}

func binaryLittleUint16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

// TestDefaultPartitionTableFormat checks the on-flash layout of the default
// partition table against the ESP-IDF format the bootloader expects.
func TestDefaultPartitionTableFormat(t *testing.T) {
	table := DefaultPartitionTable(ChipESP32S3, 16*1024*1024)

	if len(table) != ESP_PARTITION_TABLE_LEN {
		t.Fatalf("partition table length = %d, want %d", len(table), ESP_PARTITION_TABLE_LEN)
	}

	// The three real partitions must be present, 32 bytes each, with the
	// expected offsets. The factory partition is sized per espflash's
	// default_partition_table: min(flash - 0x10000, MAX_PARTITION_SIZE).
	// MAX_PARTITION_SIZE = 16*1000*1024 = 0xfa0000, which caps the 0xff0000
	// available on a 16 MB chip, so the factory is 0xfa0000.
	want := []struct {
		label string
		off   uint32
		size  uint32
	}{
		{"nvs", 0x9000, 0x6000},
		{"phy_init", 0xF000, 0x1000},
		{"factory", 0x10000, 0xfa0000},
	}
	for i, w := range want {
		off := i * ESP_PARTITION_ENTRY_SIZE
		magic := binaryLittleUint16(table[off:])
		if magic != ESP_PARTITION_MAGIC {
			t.Fatalf("entry %d magic = 0x%04x, want 0x%04x", i, magic, ESP_PARTITION_MAGIC)
		}
		gotOff := uint32(table[off+4]) | uint32(table[off+5])<<8 | uint32(table[off+6])<<16 | uint32(table[off+7])<<24
		gotSize := uint32(table[off+8]) | uint32(table[off+9])<<8 | uint32(table[off+10])<<16 | uint32(table[off+11])<<24
		if gotOff != w.off || gotSize != w.size {
			t.Fatalf("entry %d (%s): offset=0x%x size=0x%x, want offset=0x%x size=0x%x",
				i, w.label, gotOff, gotSize, w.off, w.size)
		}
	}

	// The 0xEBEB MD5 marker must sit right after the real entries, with the
	// 16-byte digest at entry+16.
	marker := 3 * ESP_PARTITION_ENTRY_SIZE
	if magic := binaryLittleUint16(table[marker:]); magic != ESP_PARTITION_MAGIC_MD5 {
		t.Fatalf("marker magic = 0x%04x, want 0x%04x", magic, ESP_PARTITION_MAGIC_MD5)
	}
	if !verifyPartitionTableMD5(table) {
		t.Fatal("partition table failed MD5 verification")
	}
}

// TestDefaultPartitionTableBootloaderVerify confirms the table passes the same
// MD5 walk the ESP32 bootloader performs.
func TestDefaultPartitionTableBootloaderVerify(t *testing.T) {
	for _, chip := range []Chip{ChipESP32, ChipESP32S2, ChipESP32S3, ChipESP32C3, ChipESP32P4} {
		table := DefaultPartitionTable(chip, 16*1024*1024)
		if !verifyPartitionTableMD5(table) {
			t.Fatalf("chip %d: partition table failed MD5 verification", chip)
		}
	}
}

// TestDefaultPartitionTableP4MatchesEspflash pins the P4 factory partition to
// espflash's default_partition_table formula:
//
//	min(flash_size - 0x10000, MAX_PARTITION_SIZE)   // MAX = 16 * 1000 * 1024
//
// which is what the ESP32-P4 bootloader expects (the old code fell into the
// 3 MB default branch and produced a 0x300000 factory partition).
func TestDefaultPartitionTableP4MatchesEspflash(t *testing.T) {
	const (
		appAddr = uint32(0x10000)
	)

	cases := []struct {
		name     string
		chip     Chip
		flashMB  uint32
		wantSize uint32
	}{
		// min(flash - 0x10000, MAX=0xfa0000): 16 MiB is capped at MAX.
		{"p4_16mb", ChipESP32P4, 16, 0xfa0000},
		{"p4_4mb", ChipESP32P4, 4, 0x3f0000},
		{"p4_8mb", ChipESP32P4, 8, 0x7f0000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table := DefaultPartitionTable(tc.chip, tc.flashMB*1024*1024)
			gotOff, gotSize := factoryEntry(t, table)
			if gotOff != appAddr {
				t.Errorf("factory offset = 0x%x, want 0x%x", gotOff, appAddr)
			}
			if gotSize != tc.wantSize {
				t.Errorf("factory size = 0x%x, want 0x%x (espflash formula)", gotSize, tc.wantSize)
			}
			if !verifyPartitionTableMD5(table) {
				t.Error("partition table failed MD5 verification")
			}
		})
	}
}

// factoryEntry returns the offset and size of the factory partition entry.
func factoryEntry(t *testing.T, table []byte) (uint32, uint32) {
	t.Helper()
	for off := 0; off+ESP_PARTITION_ENTRY_SIZE <= len(table); off += ESP_PARTITION_ENTRY_SIZE {
		// The 16-byte label field is null-padded; compare the trimmed name.
		label := strings.TrimRight(string(table[off+12:off+12+16]), "\x00")
		if label != "factory" {
			continue
		}
		size := uint32(table[off+8]) | uint32(table[off+9])<<8 | uint32(table[off+10])<<16 | uint32(table[off+11])<<24
		return uint32(table[off+4]) | uint32(table[off+5])<<8 | uint32(table[off+6])<<16 | uint32(table[off+7])<<24, size
	}
	t.Fatal("factory partition entry not found")
	return 0, 0
}
