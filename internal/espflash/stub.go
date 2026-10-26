package espflash

import (
	"embed"
	"encoding/base64"
	"fmt"

	"github.com/BurntSushi/toml"
)

//go:generate go run ../../tools/update-stubs.go
//go:embed stubs
var stubFS embed.FS

// chipStubName maps each supported chip type to its stub TOML filename stem.
var chipStubName = map[ChipType]string{
	ChipESP8266: "stub_flasher_8266",
	ChipESP32:   "stub_flasher_32",
	ChipESP32S2: "stub_flasher_32s2",
	ChipESP32S3: "stub_flasher_32s3",
	ChipESP32C2: "stub_flasher_32c2",
	ChipESP32C3: "stub_flasher_32c3",
	ChipESP32C5: "stub_flasher_32c5",
	ChipESP32C6: "stub_flasher_32c6",
	ChipESP32H2: "stub_flasher_32h2",
	ChipESP32P4: "stub_flasher_32p4",
}

// stubTOML mirrors the TOML structure of the espflash stub flasher files.
type stubTOML struct {
	Entry     uint32 `toml:"entry"`
	Text      string `toml:"text"`
	TextStart uint32 `toml:"text_start"`
	Data      string `toml:"data"`
	DataStart uint32 `toml:"data_start"`
}

// stub holds the decoded stub loader image ready for uploading.
type stub struct {
	text      []byte
	textStart uint32
	data      []byte
	dataStart uint32
	entry     uint32
}

// stubFor returns the stub loader for the given chip type.
// Returns nil, false if no stub is available for the chip.
func stubFor(chipType ChipType) (*stub, bool) {
	return stubForRevision(chipType, 0)
}

// stubForRevision returns the stub loader for the given chip type and revision.
// A revision of 0 (unknown) falls back to the default stub for the chip. Some
// chips (notably ESP32-P4) ship revision-specific stub flashers that must be
// selected by silicon revision, so the revision is honoured here.
func stubForRevision(chipType ChipType, revision int) (*stub, bool) {
	name, ok := chipStubName[chipType]
	if !ok {
		return nil, false
	}

	// Chips with revision-specific stubs override the base name when the
	// detected revision matches; otherwise we fall back to the generic stub so
	// unlisted revisions still flash.
	if override, ok := revisionStubOverride(chipType, revision); ok {
		name = override
	}

	raw, err := stubFS.ReadFile(fmt.Sprintf("stubs/%s.toml", name))
	if err != nil {
		return nil, false
	}

	var st stubTOML
	if err := toml.Unmarshal(raw, &st); err != nil {
		return nil, false
	}

	text, err := base64.StdEncoding.DecodeString(st.Text)
	if err != nil {
		return nil, false
	}

	var data []byte
	if st.Data != "" {
		data, err = base64.StdEncoding.DecodeString(st.Data)
		if err != nil {
			return nil, false
		}
	}

	return &stub{
		text:      text,
		textStart: st.TextStart,
		data:      data,
		dataStart: st.DataStart,
		entry:     st.Entry,
	}, true
}

// revisionStubOverride returns the stub filename stem to use for a chip/revision
// pair, and true when a revision-specific stub applies. It returns false so the
// caller uses the generic chip stub when no override exists.
func revisionStubOverride(chipType ChipType, revision int) (string, bool) {
	// ESP32-P4: rev >= 3.0 needs the rev3 stub (different OCRAM load address);
	// rev < 3.0 uses the rev1 stub. Both ship as separate embed files.
	if chipType == ChipESP32P4 {
		if revision >= 300 {
			return "stub_flasher_32p4_rev3", true
		}
		return "stub_flasher_32p4", true
	}
	return "", false
}
