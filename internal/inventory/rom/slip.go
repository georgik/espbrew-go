package rom

import (
	"bytes"
)

const (
	SLIP_END     = 0xC0
	SLIP_ESC     = 0xDB
	SLIP_ESC_END = 0xDC
	SLIP_ESC_ESC = 0xDD
)

// EncodeSLIP encodes data using SLIP framing per ESP ROM protocol
func EncodeSLIP(data []byte) []byte {
	var buf bytes.Buffer

	for _, b := range data {
		switch b {
		case SLIP_END:
			buf.WriteByte(SLIP_ESC)
			buf.WriteByte(SLIP_ESC_END)
		case SLIP_ESC:
			buf.WriteByte(SLIP_ESC)
			buf.WriteByte(SLIP_ESC_ESC)
		default:
			buf.WriteByte(b)
		}
	}

	buf.WriteByte(SLIP_END)
	return buf.Bytes()
}

// DecodeSLIP decodes SLIP-framed data per ESP ROM protocol
func DecodeSLIP(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	escape := false

	for _, b := range data {
		if escape {
			switch b {
			case SLIP_ESC_END:
				buf.WriteByte(SLIP_END)
			case SLIP_ESC_ESC:
				buf.WriteByte(SLIP_ESC)
			default:
				// Invalid escape sequence, keep as-is
				buf.WriteByte(b)
			}
			escape = false
			continue
		}

		switch b {
		case SLIP_END:
			// Frame boundary, continue
		case SLIP_ESC:
			escape = true
		default:
			buf.WriteByte(b)
		}
	}

	return buf.Bytes(), nil
}
