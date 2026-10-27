package espflash

import "fmt"

// p4CommandPost is the command follow-up for the ESP32-P4 ROM bootloader.
//
// The P4 ROM defers each reply until the next command arrives (one-command
// pipeline lag). In ROM mode we therefore send a SYNC "poke" so the ROM flushes
// the queued reply for the command just sent, then Drain() the write so the
// command is fully transmitted before we read. In stub mode the RAM stub replies
// immediately, so both are skipped: Draining there would block forever waiting
// for a stub that is not reading commands.
//
// See wiki/esp32-p4-flash-solution.md.
type p4CommandPost struct{}

func (p4CommandPost) apply(c *conn) error {
	if c.isStub() {
		return nil
	}
	if err := c.sendCommand(cmdSync, syncPokePayload, 0); err != nil {
		return fmt.Errorf("send sync poke: %w", err)
	}
	return c.port.Drain()
}

func init() {
	registerCommandPost(ChipESP32P4, p4CommandPost{})
}
