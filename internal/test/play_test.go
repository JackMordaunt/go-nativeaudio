package test

import (
	"testing"
	"time"

	"git.sr.ht/~jackmordaunt/nativeaudio/play"
)

// TestPlayTwice plays one second of silence twice in the same process.
//
// Each call must take at least the file's duration: returning early means
// the tail of the audio was cut off. The second call must succeed at all:
// the playback context is process-wide and used to be re-created per call,
// which the audio backend refuses.
//
// Playback goes through oto on every platform, so this test is not
// platform-specific. It skips when no audio output is available.
func TestPlayTwice(t *testing.T) {
	const seconds = 1
	path := writeTemp(t, "silence.wav", silentWAV(44100, 2, seconds))
	for i := 1; i <= 2; i++ {
		start := time.Now()
		err := play.File(path)
		elapsed := time.Since(start)
		if err != nil {
			// The first call doubles as the availability probe. CI
			// runners have no sound hardware and each backend reports
			// that differently, so any first-call failure is treated as
			// "no audio here" rather than a bug. A second-call failure
			// is the regression this test exists to catch, so it is
			// always fatal.
			if i == 1 {
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
