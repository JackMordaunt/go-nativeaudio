package test

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

// TestPlayTwice plays one second of silence twice in the same process.
//
// Each call must take at least the file's duration: returning early means
// the tail of the audio was cut off. The second call must succeed at all:
// the playback context is process-wide and used to be re-created per call,
// which the audio backend refuses.
//
// The test skips when no audio output is available.
func TestPlayTwice(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		// Other platforms play through ffplay; that path has its own test.
		if _, err := exec.LookPath("ffplay"); err != nil {
			t.Skip("ffplay not installed")
		}
	}
	const seconds = 1
	path := writeTemp(t, "silence.wav", silentWAV(44100, 2, seconds))
	for i := 1; i <= 2; i++ {
		start := time.Now()
		err := nativeaudio.Play(path)
		elapsed := time.Since(start)
		if err != nil {
			if i == 1 && strings.Contains(err.Error(), "starting playback context") {
				t.Skipf("no audio output available: %v", err)
			}
			t.Fatalf("play %d: %v", i, err)
		}
		t.Logf("play %d took %v", i, elapsed.Round(time.Millisecond))
		if min := time.Duration(seconds) * time.Second * 9 / 10; elapsed < min {
			t.Errorf("play %d returned after %v, want at least %v: audio tail was cut off", i, elapsed, min)
		}
		if max := 10 * time.Second; elapsed > max {
			t.Errorf("play %d took %v, want under %v", i, elapsed, max)
		}
	}
}
