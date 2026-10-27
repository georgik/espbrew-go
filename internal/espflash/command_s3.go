package espflash

// s3CommandPost is the command follow-up for the ESP32-S3 family. It inherits
// the plain immediate-reply behavior (S3 answers each command directly, so no
// poke and no Drain are needed). It exists as its own type and file so the S3
// family can diverge independently later (for example a derived chip such as
// ESP32-S31) without touching the base or the P4 path.
type s3CommandPost struct {
	plainCommandPost
}

func init() {
	registerCommandPost(ChipESP32S3, s3CommandPost{})
}
