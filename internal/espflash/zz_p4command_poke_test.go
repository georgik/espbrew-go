package espflash

import (
	"context"
	"encoding/binary"
	"testing"
	"time"
)

// pokeMock is a serial.Port mock that records writes, counts Drain() calls, and
// replays a fixed queue of SLIP frames on Read (then reports "no data" to
// simulate a read timeout). It lets us exercise conn.command()'s P4
// pipeline-lag handling without real hardware.
type pokeMock struct {
	mockPort
	writes    [][]byte
	drainSeen bool
	readQ     [][]byte
	readIdx   int
}

func (p *pokeMock) Write(b []byte) (int, error) {
	cp := make([]byte, len(b))
	copy(cp, b)
	p.writes = append(p.writes, cp)
	return len(b), nil
}

func (p *pokeMock) Read(b []byte) (int, error) {
	if p.readIdx >= len(p.readQ) {
		return 0, nil // no more queued frames -> read timeout (n==0)
	}
	n := copy(b, p.readQ[p.readIdx])
	p.readIdx++
	return n, nil
}

func (p *pokeMock) Drain() error {
	p.drainSeen = true
	return nil
}

// slipResp builds a SLIP-framed ROM response for the given opcode/value so a
// test can queue realistic replies.
func slipResp(opcode byte, value uint32, data ...byte) []byte {
	f := []byte{respDirectionResp, opcode, 0x00, 0x00}
	v := make([]byte, 4)
	binary.LittleEndian.PutUint32(v, value)
	f = append(f, v...)
	f = append(f, data...)
	f = append(f, 0x00, 0x00) // 2 status bytes (0 = OK)
	return slipEncode(f)
}

// pokeOpcode returns the opcode byte from a recorded SLIP frame: skip the
// leading SLIP END delimiter, then the first decoded byte is the request
// direction and the second is the opcode.
func pokeOpcode(frame []byte) byte {
	i := 0
	for i < len(frame) && frame[i] == slipEnd {
		i++
	}
	if i+1 < len(frame) {
		return frame[i+1] // direction=frame[i], opcode=frame[i+1]
	}
	return 0
}

// findWrite returns the first recorded write frame whose decoded opcode matches
// opcode. conn.command() sends a SYNC "poke" after each real command, so tests
// that capture writes must select the real command frame rather than the last
// (poke) write.
func findWrite(writes [][]byte, opcode byte) []byte {
	for _, w := range writes {
		i := 0
		for i < len(w) && w[i] == slipEnd {
			i++
		}
		if i+1 < len(w) && w[i+1] == opcode {
			return w
		}
	}
	return nil
}

// TestCommandSendsSyncPokeAndDrainsInROM verifies the P4 fix: after the real
// command, conn.command() sends a follow-up SYNC "poke" (so the P4 ROM flushes
// its deferred reply) and, in ROM mode, Drain()s the write so the command is
// fully transmitted before reading.
func TestCommandSendsSyncPokeAndDrainsInROM(t *testing.T) {
	m := &pokeMock{
		readQ: [][]byte{
			slipResp(cmdSync, 0),         // stale: poke reply that arrived first
			slipResp(cmdReadReg, 0x1234), // matching response we wait for
		},
	}
	c := &conn{port: m, reader: newSlipReader(m, context.Background()), post: p4CommandPost{}, stub: false}

	resp, err := c.command(cmdReadReg, []byte{0x00, 0x00, 0x00, 0x00}, 0, defaultTimeout, true)
	if err != nil {
		t.Fatalf("command() failed: %v", err)
	}
	if resp.Value != 0x1234 {
		t.Errorf("response value = 0x%08X, want 0x1234", resp.Value)
	}

	// Exactly two writes: the real command, then the SYNC poke.
	if len(m.writes) != 2 {
		t.Fatalf("expected 2 writes (command + poke), got %d", len(m.writes))
	}
	if got := pokeOpcode(m.writes[0]); got != cmdReadReg {
		t.Errorf("write[0] opcode = 0x%02X, want cmdReadReg", got)
	}
	if got := pokeOpcode(m.writes[1]); got != cmdSync {
		t.Errorf("write[1] opcode = 0x%02X, want cmdSync (poke)", got)
	}
	// ROM mode must Drain() so the command is fully transmitted before reading.
	if !m.drainSeen {
		t.Errorf("Drain() not called in ROM mode")
	}
}

// TestCommandSkipsDrainInStubMode verifies that in stub mode we do NOT Drain()
// (the RAM flash stub is not yet reading commands, so Drain()/tcdrain would
// block forever), matching espflash's behaviour.
func TestCommandSkipsDrainInStubMode(t *testing.T) {
	m := &pokeMock{
		readQ: [][]byte{
			slipResp(cmdReadReg, 0xABCD),
		},
	}
	c := &conn{port: m, reader: newSlipReader(m, context.Background()), post: p4CommandPost{}, stub: true}

	if _, err := c.command(cmdReadReg, []byte{0, 0, 0, 0}, 0, defaultTimeout, true); err != nil {
		t.Fatalf("command() failed: %v", err)
	}
	if m.drainSeen {
		t.Errorf("Drain() must NOT be called in stub mode")
	}
	// No SYNC poke in stub mode: the RAM stub replies immediately, so the P4
	// pipeline-lag poke is skipped. Only the command frame is written.
	if len(m.writes) != 1 {
		t.Errorf("expected 1 write (command only) in stub mode, got %d", len(m.writes))
	}
	if got := pokeOpcode(m.writes[0]); got != cmdReadReg {
		t.Errorf("write[0] opcode = 0x%02X, want cmdReadReg", got)
	}
}

// TestCommandDrainsStaleBeforeMatching verifies that stale frames (poke
// replies, priming-sync leftovers) are drained before the matching response is
// returned, and that resolution stays within the wall-clock deadline.
func TestCommandDrainsStaleBeforeMatching(t *testing.T) {
	const staleCount = 12
	readQ := make([][]byte, 0, staleCount+1)
	for i := 0; i < staleCount; i++ {
		readQ = append(readQ, slipResp(cmdSync, 0)) // stale poke replies
	}
	readQ = append(readQ, slipResp(cmdReadReg, 0x5678)) // the matching reply

	m := &pokeMock{readQ: readQ}
	c := &conn{port: m, reader: newSlipReader(m, context.Background()), post: p4CommandPost{}, stub: false}

	start := time.Now()
	resp, err := c.command(cmdReadReg, []byte{0, 0, 0, 0}, 0, defaultTimeout, true)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("command() failed: %v", err)
	}
	if resp.Value != 0x5678 {
		t.Errorf("response value = 0x%08X, want 0x5678", resp.Value)
	}
	// The 12 stale frames + matching reply should all be consumed.
	if m.readIdx != staleCount+1 {
		t.Errorf("read %d frames, want %d", m.readIdx, staleCount+1)
	}
	// Must resolve well within the 8 s wall-clock deadline.
	if elapsed > p4CommandDeadline {
		t.Errorf("command() took %v, exceeds deadline %v", elapsed, p4CommandDeadline)
	}
}

// TestCommandSilentChipFailsFast verifies that when the chip never answers,
// command() gives up quickly instead of looping forever. A silent chip only
// produces read timeouts, so command() bails after a few back-to-back timeouts
// (p4MaxSilentReads); the deadline is shortened here only so the test stays
// fast instead of blocking on the full 5 s production value.
func TestCommandSilentChipFailsFast(t *testing.T) {
	// Temporarily shorten the deadline and always restore it.
	saved := p4CommandDeadline
	p4CommandDeadline = 40 * time.Millisecond
	defer func() { p4CommandDeadline = saved }()

	m := &pokeMock{} // readQ empty -> every Read reports timeout (n==0)
	c := &conn{port: m, reader: newSlipReader(m, context.Background()), post: p4CommandPost{}, stub: false}

	start := time.Now()
	_, err := c.command(cmdReadReg, []byte{0, 0, 0, 0}, 0, defaultTimeout, true)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("command() should fail on a silent chip")
	}
	if elapsed > p4CommandDeadline+time.Second {
		t.Errorf("command() took %v, should fail fast near deadline %v", elapsed, p4CommandDeadline)
	}
}
