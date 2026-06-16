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
	ChipESP8266:     "stub_flasher_8266",
	ChipESP32:       "stub_flasher_32",
	ChipESP32S2:     "stub_flasher_32s2",
	ChipESP32S3:     "stub_flasher_32s3",
	ChipESP32C2:     "stub_flasher_32c2",
	ChipESP32C3:     "stub_flasher_32c3",
	ChipESP32C5:     "stub_flasher_32c5",
	ChipESP32C6:     "stub_flasher_32c6",
	ChipESP32H2:     "stub_flasher_32h2",
	ChipESP32P4Rev1: "stub_flasher_32p4",
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
	name, ok := chipStubName[chipType]
	if !ok {
		return nil, false
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
