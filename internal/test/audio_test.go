package test

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

var (
	//go:embed compressed.m4a
	compressed []byte
	//go:embed uncompressed.s16le.pcm
	uncompressed []byte
	//go:embed uncompressed.s16le.macos.pcm
	uncompressed_macos []byte
	//go:embed corrupt.m4a
	corrupt []byte
)

// getUncompressed returns the uncompressed result produced on the
// current platform.
func getUncompressed() []byte {
	switch runtime.GOOS {
	case "darwin":
		return uncompressed_macos
	default:
		return uncompressed
	}
}

// TestLoad ensures that output from the native decoders are close to
// the output of ffmpeg.
func TestLoad(t *testing.T) {
	by, f, err := newDecoder(t).DecodeFile("compressed.m4a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("format: %+v", f)
	// Check for known meta data values (ffprobe -i compressed.m4a).
	if f.BytesPerSample != 2 {
		t.Fatalf("unexpected bit depth: want 2, got %d", f.BytesPerSample)
	}
	if f.SampleRate != 44100 {
		t.Fatalf("unexpected sample rate: want 44100, got %d", f.SampleRate)
	}
	if f.Channels != 2 {
		t.Fatalf("unexpected channel count: want 2, got %d", f.Channels)
	}
	// Test passes on exact match, otherwise do a tolerance test.
	if bytes.Equal(by, getUncompressed()) {
		return
	}
	if !equal(t, by, getUncompressed()) {
		t.Fatalf("native output does not match ffmpeg output")
	}
}

// TestDecode ensures that output from the native decoders are similar to
// the output of ffmpeg.
func TestDecode(t *testing.T) {
	by, f, err := newDecoder(t).Decode(compressed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("format: %+v", f)
	// Check for known meta data values (ffprobe -i compressed.m4a).
	if f.BytesPerSample != 2 {
		t.Fatalf("unexpected bit depth: want 2, got %d", f.BytesPerSample)
	}
	if f.SampleRate != 44100 {
		t.Fatalf("unexpected sample rate: want 44100, got %d", f.SampleRate)
	}
	if f.Channels != 2 {
		t.Fatalf("unexpected channel count: want 2, got %d", f.Channels)
	}
	// Test passes on exact match, otherwise do a tolerance test.
	if bytes.Equal(by, getUncompressed()) {
		return
	}
	if !equal(t, by, getUncompressed()) {
		t.Fatalf("native output does not match ffmpeg output")
	}
}

// TestDecodeCorrupt ensures that we get an error value on invalid input and
// that we don't crash the process.
func TestDecodeCorrupt(t *testing.T) {
	_, f, err := newDecoder(t).Decode(corrupt)
	if err == nil {
		t.Fatalf("expected error for corrupt audio data, got nil")
	}
	t.Logf("format: %+v", f)
}

// TestMemoryLeak runs the decode several times, forces a GC and verifies
// that no data is left over from this package.
func TestMemoryLeak(t *testing.T) {
	runtime.MemProfileRate = 1

	// Scoped so the Decoder itself is unreachable before the profile is
	// taken. Holding it live would show up here as an allocation that
	// was never freed, which is exactly what this test looks for.
	func() {
		d, err := nativeaudio.New()
		if err != nil {
			t.Fatalf("creating decoder: %v", err)
		}
		defer d.Close()
		for ii := 0; ii < 10; ii++ {
			by, f, err := d.DecodeFile("compressed.m4a")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_ = by
			_ = f
		}
	}()

	runtime.GC()
	runtime.GC()

	var profiles []runtime.MemProfileRecord

	for {
		n, ok := runtime.MemProfile(profiles, false)
		if ok {
			profiles = profiles[:n]
			break
		}
		profiles = slices.Grow(profiles, n)[:n]
	}

	for _, p := range profiles {
		f := runtime.FuncForPC(p.Stack0[0])

		if !strings.Contains(f.Name(), "nativeaudio") {
			continue
		}

		t.Errorf("un-freed data: %s -> %d\n", f.Name(), p.InUseBytes())
	}
}

// equal decodes the PCM samples and tests if they are "close enough"
// using a heuristic tolerance.
//
// The two decoders must agree on length exactly. Sample values are
// compared as signed integers by mean absolute difference, which must
// stay under one quantisation step (1 LSB). Different AAC decoders
// legitimately differ by rounding, so bit-exact output is not expected.
func equal(t *testing.T, left, right []byte) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	if len(left) != len(right) {
		t.Logf("length mismatch: native %d bytes, reference %d bytes", len(left), len(right))
		return false
	}
	var (
		lsamples = make([]int16, len(left)/2)
		rsamples = make([]int16, len(right)/2)
	)
	if err := binary.Read(bytes.NewReader(left), binary.LittleEndian, lsamples); err != nil {
		panic(fmt.Errorf("left: binary read: %w", err))
	}
	if err := binary.Read(bytes.NewReader(right), binary.LittleEndian, rsamples); err != nil {
		panic(fmt.Errorf("right: binary read: %w", err))
	}
	var (
		size int = min(len(lsamples), len(rsamples))
		sum  int = 0
	)
	for ii := 0; ii < size; ii++ {
		var (
			lsample = int(lsamples[ii])
			rsample = int(rsamples[ii])
		)
		sum += abs(lsample - rsample)
	}
	mean := float64(sum) / float64(size)
	t.Logf("mean: %f, sum: %d, size: %d\n", mean, sum, size)
	return mean < 1.0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func BenchmarkDecode(b *testing.B) {
	b.Run("native-decode", func(b *testing.B) {
		d := newDecoder(b)
		b.ResetTimer()
		for ii := 0; ii < b.N; ii++ {
			by, f, err := d.Decode(compressed)
			if err != nil {
				b.Fatalf("unexpected error during decode: %v", err)
			}
			_ = by
			_ = f
		}
	})
	b.Run("ffmpeg-decode", func(b *testing.B) {
		for ii := 0; ii < b.N; ii++ {
			by, f, err := nativeaudio.FFmpegDecode(compressed)
			if err != nil {
				b.Fatalf("unexpected error during decode: %v", err)
			}
			_ = by
			_ = f
		}
	})
}
