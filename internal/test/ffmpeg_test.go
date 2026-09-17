package test

import (
	"bytes"
	"os/exec"
	"testing"
	"time"

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
	if !bytes.Equal(loaded, uncompressed) && !equal(t, loaded, uncompressed) {
		t.Fatal("FFmpegLoad output does not match reference")
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

// TestFFmpegPlay plays one second of silence through ffplay and checks
// the call blocks for at least that long. A mistyped flag used to make
// ffplay exit immediately with an error.
func TestFFmpegPlay(t *testing.T) {
	requireTools(t, "ffplay")
	const seconds = 1
	path := writeTemp(t, "silence.wav", silentWAV(44100, 2, seconds))
	start := time.Now()
	if err := nativeaudio.FFmpegPlay(path); err != nil {
		t.Fatalf("FFmpegPlay: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("ffplay took %v", elapsed.Round(time.Millisecond))
	if min := time.Duration(seconds) * time.Second * 9 / 10; elapsed < min {
		t.Errorf("FFmpegPlay returned after %v, want at least %v", elapsed, min)
	}
}
