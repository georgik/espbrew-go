package rom

// Chip magic values from strapping pins/eFuse
const (
	ESP32_MAGIC        = 0x00f01d83
	ESP32S2_MAGIC      = 0x000007c6
	ESP32S3_MAGIC      = 0x09
	ESP32C3_MAGIC      = 0x6921506f // ECO1+2
	ESP32C3_MAGIC_ECO3 = 0x1b31506f
	ESP32C3_MAGIC_ECO6 = 0x4881606f
	ESP32C6_MAGIC      = 0x2ce0806f
)

// Security info chip-id values (from GET_SECURITY_INFO response)
const (
	SECURITY_ID_ESP32S2 = 0x2FCD81BF
	SECURITY_ID_ESP32S3 = 0x09
	SECURITY_ID_ESP32C3 = 0x1B31506F
	SECURITY_ID_ESP32C6 = 0x2CE0806F
	SECURITY_ID_ESP32H2 = 0xD7B73E80
	SECURITY_ID_ESP32P4 = 0x0ADDBAD0
)

// chipByMagic maps magic values to Chip implementations
var chipByMagic = map[uint32]func() Chip{
	ESP32S3_MAGIC: func() Chip { return &ESP32S3Chip{} },
	ESP32C3_MAGIC: func() Chip { return &ESP32C3Chip{} },
	ESP32C6_MAGIC: func() Chip { return &ESP32C6Chip{} },
}

// chipBySecurityID maps security-id values to Chip implementations
var chipBySecurityID = map[uint32]func() Chip{
	SECURITY_ID_ESP32S3: func() Chip { return &ESP32S3Chip{} },
	SECURITY_ID_ESP32S2: func() Chip { return &ESP32S2Chip{} },
	SECURITY_ID_ESP32C3: func() Chip { return &ESP32C3Chip{} },
	SECURITY_ID_ESP32C6: func() Chip { return &ESP32C6Chip{} },
	SECURITY_ID_ESP32H2: func() Chip { return &ESP32H2Chip{} },
	SECURITY_ID_ESP32P4: func() Chip { return &ESP32P4Chip{} },
}

// DetectByMagic detects chip by magic register value
func DetectByMagic(magic uint32) Chip {
	// Try exact match first
	if factory, ok := chipByMagic[magic]; ok {
		return factory()
	}

	// Try ESP32-C3 alternate magic values
	switch magic {
	case ESP32C3_MAGIC_ECO3, ESP32C3_MAGIC_ECO6:
		return &ESP32C3Chip{}
	}

	return nil
}

// DetectBySecurityID detects chip by GET_SECURITY_INFO chip-id
func DetectBySecurityID(chipID uint32) Chip {
	if factory, ok := chipBySecurityID[chipID]; ok {
		return factory()
	}
	return nil
}
