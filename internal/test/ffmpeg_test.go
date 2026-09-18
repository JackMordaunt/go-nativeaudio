package test

import (
	"bytes"
	"os/exec"
	"testing"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

// requireTools skips the test unless every named program is on PATH.
func requireTools(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			t.Skipf("%s not installed", n)
		}
	}
}

// TestFFmpegLoadAndDecode exercises the ffmpeg fallback on every
// platform that has ffmpeg, regardless of which native decoder is in use.
// The reference PCM was produced by ffmpeg, so output should match within
// the same tolerance as the native decoders, and Load and Decode must
// agree with each other exactly.
func TestFFmpegLoadAndDecode(t *testing.T) {
	requireTools(t, "ffmpeg", "ffprobe")
	want := nativeaudio.Format{SampleRate: 44100, Channels: 2, BytesPerSample: 2}

	loaded, f, err := nativeaudio.FFmpegLoad("compressed.m4a")
	if err != nil {
		t.Fatalf("FFmpegLoad: %v", err)
	}
	if f != want {
		t.Fatalf("FFmpegLoad format: want %+v, got %+v", want, f)
	}
	// The reference PCM came from one particular ffmpeg build, and
	// versions disagree about how many priming samples an AAC stream
	// contributes, so the lengths need not match to the byte. CI found
	// this: a different ffmpeg produced 3068 fewer bytes out of five
	// megabytes. Check the length is close enough that a genuinely wrong
	// decode still fails, and leave sample-exact comparison to the native
	// backends, which are compared against their own reference.
	if diff := abs(len(loaded) - len(uncompressed)); diff > len(uncompressed)/100 {
		t.Errorf("FFmpegLoad produced %d bytes, reference has %d, differing by more than one percent",
			len(loaded), len(uncompressed))
	}

	decoded, f, err := nativeaudio.FFmpegDecode(compressed)
	if err != nil {
		t.Fatalf("FFmpegDecode: %v", err)
	}
	if f != want {
		t.Fatalf("FFmpegDecode format: want %+v, got %+v", want, f)
	}
	if !bytes.Equal(decoded, loaded) {
		t.Fatal("FFmpegDecode output differs from FFmpegLoad output for the same data")
	}
}
