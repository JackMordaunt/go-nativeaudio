package test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

// silentWAV builds a canonical 16-bit PCM WAV of digital silence in
// memory, so format tests need no fixtures and playback tests make no
// sound.
func silentWAV(rate, channels, seconds int) []byte {
	n := rate * channels * 2 * seconds
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+n))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))              // fmt chunk size
	binary.Write(&b, binary.LittleEndian, uint16(1))               // PCM
	binary.Write(&b, binary.LittleEndian, uint16(channels))        // channels
	binary.Write(&b, binary.LittleEndian, uint32(rate))            // sample rate
	binary.Write(&b, binary.LittleEndian, uint32(rate*channels*2)) // byte rate
	binary.Write(&b, binary.LittleEndian, uint16(channels*2))      // block align
	binary.Write(&b, binary.LittleEndian, uint16(16))              // bits per sample
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(n))
	b.Write(make([]byte, n))
	return b.Bytes()
}

// writeTemp writes data to a file in the test's temp dir and returns its path.
func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

// TestDecodeFormats checks that Format reports the source's sample rate
// and channel count for inputs other than the 44.1 kHz stereo fixture,
// and that the PCM length matches.
func TestDecodeFormats(t *testing.T) {
	cases := []struct{ rate, channels int }{
		{44100, 2},
		{22050, 1},
		{48000, 1},
		{48000, 2},
	}
	for _, c := range cases {
		by, f, err := newDecoder(t).Decode(silentWAV(c.rate, c.channels, 1))
		if err != nil {
			t.Errorf("%d Hz %d ch: unexpected error: %v", c.rate, c.channels, err)
			continue
		}
		want := nativeaudio.Format{SampleRate: c.rate, Channels: c.channels, BytesPerSample: 2}
		if f != want {
			t.Errorf("%d Hz %d ch: format: want %+v, got %+v", c.rate, c.channels, want, f)
		}
		if wantLen := c.rate * c.channels * 2; len(by) != wantLen {
			t.Errorf("%d Hz %d ch: pcm length: want %d, got %d", c.rate, c.channels, wantLen, len(by))
		}
	}
}

// TestLoadMissingFile ensures a bad path is reported as an error.
func TestLoadMissingFile(t *testing.T) {
	if _, _, err := newDecoder(t).DecodeFile(filepath.Join(t.TempDir(), "does-not-exist.m4a")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestStartEndCycle ensures the platform can be torn down and brought
// back up repeatedly, with a decode in between to prove each Start took.
func TestDecoderLifecycle(t *testing.T) {
	// Several create-and-close cycles, with a decode in between to prove
	// each New actually initialised the platform.
	for i := 0; i < 3; i++ {
		d, err := nativeaudio.New()
		if err != nil {
			t.Fatalf("cycle %d: New: %v", i, err)
		}
		if _, _, err := d.Decode(compressed); err != nil {
			t.Fatalf("cycle %d: Decode: %v", i, err)
		}
		if err := d.Close(); err != nil {
			t.Fatalf("cycle %d: Close: %v", i, err)
		}
		if err := d.Close(); err != nil {
			t.Fatalf("cycle %d: second Close should be a no-op: %v", i, err)
		}
		if _, _, err := d.Decode(compressed); !errors.Is(err, nativeaudio.ErrClosed) {
			t.Fatalf("cycle %d: decode after close: want ErrClosed, got %v", i, err)
		}
	}
}

// TestDecodersAreIndependent ensures closing one Decoder does not tear
// the platform out from under another. This is the whole reason the API
// is a value rather than a set of package functions.
func TestDecodersAreIndependent(t *testing.T) {
	a, err := nativeaudio.New()
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	b := newDecoder(t)
	if err := a.Close(); err != nil {
		t.Fatalf("closing first: %v", err)
	}
	if _, _, err := b.Decode(compressed); err != nil {
		t.Fatalf("second decoder broke when the first closed: %v", err)
	}
}

// TestDecodeCorruptMidStream overwrites a stretch of sample data in the
// middle of the fixture. Decoders differ in whether they conceal the
// damage or fail, so either a result or an error is acceptable. What is
// not acceptable is a hang: a decoder that returns no sample and no
// end-of-stream flag on error must be treated as terminal.
func TestDecodeCorruptMidStream(t *testing.T) {
	bad := append([]byte(nil), compressed...)
	mid := len(bad) / 2
	for i := mid; i < mid+64*1024 && i < len(bad); i++ {
		bad[i] = 0xff
	}
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		by, _, err := newDecoder(t).Decode(bad)
		done <- result{len(by), err}
	}()
	select {
	case r := <-done:
		t.Logf("mid-stream corruption: %d bytes, err=%v", r.n, r.err)
	case <-time.After(30 * time.Second):
		t.Fatal("decode did not return within 30s")
	}
}

// newDecoder returns a Decoder that is closed when the test ends.
func newDecoder(t testing.TB) *nativeaudio.Decoder {
	t.Helper()
	d, err := nativeaudio.New()
	if err != nil {
		t.Fatalf("creating decoder: %v", err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Errorf("closing decoder: %v", err)
		}
	})
	return d
}
