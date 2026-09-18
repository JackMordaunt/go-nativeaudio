package play_test

import (
	"testing"
	"time"

	"git.sr.ht/~jackmordaunt/nativeaudio"
	"git.sr.ht/~jackmordaunt/nativeaudio/play"
)

// TestDuration covers the calculation that bounds the playback wait, so
// a mistake there cannot quietly reintroduce an unbounded one.
func TestDuration(t *testing.T) {
	stereo := nativeaudio.Format{SampleRate: 44100, Channels: 2, BytesPerSample: 2}
	mono := nativeaudio.Format{SampleRate: 8000, Channels: 1, BytesPerSample: 2}

	for _, tc := range []struct {
		name   string
		bytes  int
		format nativeaudio.Format
		want   time.Duration
	}{
		{"one second stereo", 44100 * 2 * 2, stereo, time.Second},
		{"half a second stereo", 44100 * 2, stereo, time.Second / 2},
		{"one second mono", 8000 * 2, mono, time.Second},
		{"nothing", 0, stereo, 0},
		{"degenerate format", 1024, nativeaudio.Format{}, 0},
	} {
		got := play.Duration(make([]byte, tc.bytes), tc.format)
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
