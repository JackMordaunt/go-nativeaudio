package test

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"testing"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

var (
	//go:embed compressed.m4a
	compressed []byte
	//go:embed uncompressed.s16le.pcm
	uncompressed []byte
	//go:embed corrupt.m4a
	corrupt []byte
)

// TestLoad ensures that output from the native decoders are close to
// the output of ffmpeg.
func TestLoad(t *testing.T) {
	by, f, err := nativeaudio.Load("compressed.m4a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("format: %+v", f)
	// Check for known meta data values (ffprobe -i compressed.m4a).
	if f.BitDepth != 2 {
		t.Fatalf("unexpected bit depth: want 2, got %d", f.BitDepth)
	}
	if f.SampleRate != 44100 {
		t.Fatalf("unexpected sample rate: want 44100, got %d", f.SampleRate)
	}
	if f.Channels != 2 {
		t.Fatalf("unexpected channel count: want 2, got %d", f.Channels)
	}
	// Test passes on exact match, otherwise do a tolerance test.
	if bytes.Equal(by, uncompressed) {
		return
	}
	if !equal(t, by, uncompressed) {
		t.Fatalf("native output does not match ffmpeg output")
	}
}

// TestDecode ensures that output from the native decoders are similar to
// the output of ffmpeg.
func TestDecode(t *testing.T) {
	by, f, err := nativeaudio.Decode(compressed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("format: %+v", f)
	// Check for known meta data values (ffprobe -i compressed.m4a).
	if f.BitDepth != 2 {
		t.Fatalf("unexpected bit depth: want 2, got %d", f.BitDepth)
	}
	if f.SampleRate != 44100 {
		t.Fatalf("unexpected sample rate: want 44100, got %d", f.SampleRate)
	}
	if f.Channels != 2 {
		t.Fatalf("unexpected channel count: want 2, got %d", f.Channels)
	}
	// Test passes on exact match, otherwise do a tolerance test.
	if bytes.Equal(by, uncompressed) {
		return
	}
	if !equal(t, by, uncompressed) {
		t.Fatalf("native output does not match ffmpeg output")
	}
}

// TestDecodeCorrupt ensures that we get an error value on invalid input and
// that we don't crash the process.
func TestDecodeCorrupt(t *testing.T) {
	_, f, err := nativeaudio.Decode(corrupt)
	if err == nil {
		t.Fatalf("expected error for corrupt audio data, got nil")
	}
	t.Logf("format: %+v", f)
}

// equal decodes the PCM samples and tests if they are "close enough"
// using a heuristic tolerance.
//
// Decode each sample as a signed integer and compute the absolute
// difference on average.
func equal(t *testing.T, left, right []byte) bool {
	if len(left) == 0 || len(right) == 0 {
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
	if mean > 0.1 {
		return false
	}
	return true
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func BenchmarkDecode(b *testing.B) {
	b.Run("native-load", func(b *testing.B) {
		for ii := 0; ii < b.N; ii++ {
			by, f, err := nativeaudio.Load("compressed.m4a")
			if err != nil {
				b.Fatalf("unexpected error during decode: %v", err)
			}
			_ = by
			_ = f
		}
	})
	b.Run("native-decode", func(b *testing.B) {
		for ii := 0; ii < b.N; ii++ {
			by, f, err := nativeaudio.Decode(compressed)
			if err != nil {
				b.Fatalf("unexpected error during decode: %v", err)
			}
			_ = by
			_ = f
		}
	})
	b.Run("ffmpeg-load", func(b *testing.B) {
		for ii := 0; ii < b.N; ii++ {
			by, f, err := nativeaudio.FFmpegLoad("compressed.m4a")
			if err != nil {
				b.Fatalf("unexpected error during decode: %v", err)
			}
			_ = by
			_ = f
		}
	})
}
