package test

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

// checkFormat asserts that a decode which reported success produced a
// self-consistent result. A decoder is free to reject anything it does
// not like, but if it claims to have decoded something then the format
// has to describe the PCM it handed back.
func checkFormat(t *testing.T, pcm []byte, f nativeaudio.Format) {
	t.Helper()
	if f.BytesPerSample != 2 {
		t.Errorf("decode succeeded with BytesPerSample %d, want 2", f.BytesPerSample)
	}
	if f.Channels < 1 {
		t.Errorf("decode succeeded with %d channels", f.Channels)
	}
	if f.SampleRate < 1 {
		t.Errorf("decode succeeded with sample rate %d", f.SampleRate)
	}
	if f.Channels < 1 || f.BytesPerSample < 1 {
		return
	}
	if frame := f.Channels * f.BytesPerSample; len(pcm)%frame != 0 {
		t.Errorf("decode produced %d bytes, not a whole number of %d-byte frames", len(pcm), frame)
	}
}

// fuzzDecoder returns a Decoder on a short budget, closed when the fuzz
// target finishes.
//
// Fuzzing needs every input to finish quickly. Malformed audio can make a
// decoder grind for far longer than the harness tolerates before it
// declares a worker hung, so the budget here is much tighter than the
// default a normal caller gets.
func fuzzDecoder(f *testing.F) *nativeaudio.Decoder {
	f.Helper()
	d, err := nativeaudio.New(nativeaudio.WithLimits(nativeaudio.Limits{
		MaxDuration: 250 * time.Millisecond,
		MaxBytes:    32 << 20,
	}))
	if err != nil {
		f.Fatalf("creating decoder: %v", err)
	}
	f.Cleanup(func() { d.Close() })
	return d
}

// seed adds the corpus shared by the fuzz targets: real files, a known
// corrupt one, generated PCM, and the degenerate shapes that tend to
// find off-by-one handling in header parsers.
func seed(f *testing.F) {
	f.Add(compressed)
	f.Add(corrupt)
	f.Add(uncompressed[:4096]) // a slice: seeds want to be small
	f.Add(silentWAV(44100, 2, 1))
	f.Add(silentWAV(8000, 1, 1))
	f.Add([]byte{})
	f.Add([]byte("RIFF"))
	f.Add([]byte("RIFF\x00\x00\x00\x00WAVEfmt "))
	f.Add([]byte("\x00\x00\x00\x20ftypM4A "))
}

// FuzzDecode feeds arbitrary bytes to the platform decoder.
//
// This is the package's main untrusted-input surface: callers hand it
// files they did not create, and on Windows and macOS those bytes reach
// the operating system's own decoder through hand-written bindings. The
// property under test is that decoding either fails or returns a
// coherent result, and never panics or corrupts memory.
func FuzzDecode(f *testing.F) {
	seed(f)

	d := fuzzDecoder(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		pcm, format, err := d.Decode(data)
		if err != nil {
			return
		}
		checkFormat(t, pcm, format)
	})
}

// FuzzStream feeds arbitrary bytes to the incremental decoder, which
// has its own bookkeeping around sample lifetime and buffer locking
// that the buffered path does not exercise on the same schedule.
func FuzzStream(f *testing.F) {
	seed(f)

	d := fuzzDecoder(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := d.Stream(data)
		if err != nil {
			return
		}
		defer s.Close()

		format := s.Format()
		pcm, err := io.ReadAll(s)
		if err != nil {
			return
		}
		checkFormat(t, pcm, format)
	})
}

// FuzzStreamAbandoned closes streams part-way through, which is the
// path that leaves decoder state and, where ffmpeg is the backend, a
// live subprocess to clean up.
func FuzzStreamAbandoned(f *testing.F) {
	seed(f)

	d := fuzzDecoder(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := d.Stream(data)
		if err != nil {
			return
		}
		if _, err := io.CopyN(io.Discard, s, 64); err != nil && err != io.EOF {
			// A read error is an acceptable outcome for junk input.
			_ = s.Close()
			return
		}
		if err := s.Close(); err != nil {
			t.Errorf("closing abandoned stream: %v", err)
		}
	})
}

// TestPathologicalDecodeIsBounded pins down the fix for a decode that
// used to be unrecoverable.
//
// testdata/pathological.m4a is a mutated copy of the real fixture, found by
// FuzzDecode. It opens cleanly and reports a plausible format, then
// Media Foundation decodes it indefinitely: a few hundred kilobytes of
// PCM at a time, never finishing and never failing. Every individual
// read completes, so no per-read timeout catches it. Left alone it runs
// for hours.
//
// Limits are what bound it, so the decode fails instead of running
// forever.
func TestPathologicalDecodeIsBounded(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("testdata", "pathological.m4a"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	const budget = 5 * time.Second
	d, err := nativeaudio.New(nativeaudio.WithLimits(nativeaudio.Limits{MaxDuration: budget}))
	if err != nil {
		t.Fatalf("creating decoder: %v", err)
	}
	defer d.Close()

	start := time.Now()
	_, _, err = d.Decode(in)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("decode of a pathological file succeeded after %v", elapsed)
	}
	// Two things can end it: the configured budget, checked between
	// reads, or a single read stalling past the backend's own timeout.
	// Which one wins depends on where the decoder gets stuck, and the
	// guarantee being pinned down here is termination, not the route.
	if max := budget + 30*time.Second; elapsed > max {
		t.Errorf("decode took %v, want under %v", elapsed, max)
	}
	t.Logf("bounded after %v: %v", elapsed.Round(time.Millisecond), err)
}
