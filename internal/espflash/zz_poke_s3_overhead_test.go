package espflash

import (
	"context"
	"testing"
)

// pokeCountInMode counts how many SYNC pokes command() emits while issuing
// `cmds` flash commands, given the connection's stub state.
func pokeCountInMode(t *testing.T, stub bool, cmds int) int {
	t.Helper()
	m := &pokeMock{}
	c := &conn{port: m, reader: newSlipReader(m, context.Background()), post: p4CommandPost{}, stub: stub}
	for i := 0; i < cmds; i++ {
		m.readQ = append(m.readQ, slipResp(cmdFlashData, 0))
	}
	for i := 0; i < cmds; i++ {
		if _, err := c.command(cmdFlashData, []byte{0x01, 0x02, 0x03, 0x04}, 0, defaultTimeout, true); err != nil {
			t.Fatalf("command %d: %v", i, err)
		}
	}
	pokes := 0
	for _, w := range m.writes {
		if pokeOpcode(w) == cmdSync {
			pokes++
		}
	}
	return pokes
}

// TestPokeOnlyInROMMode verifies the SYNC poke is emitted only for ROM
// bootloaders (the one-command pipeline lag), never for the RAM stub. This is
// the guard against the poke leaking onto stub-based chips such as ESP32-S3,
// which would double write traffic and slow flashing.
func TestPokeOnlyInROMMode(t *testing.T) {
	const cmds = 20

	if got := pokeCountInMode(t, true, cmds); got != 0 {
		t.Errorf("stub-mode sync pokes = %d, want 0 (must not poke while stub runs)", got)
	}
	if got := pokeCountInMode(t, false, cmds); got != cmds {
		t.Errorf("ROM-mode sync pokes = %d, want %d (one poke per command)", got, cmds)
	}
}
