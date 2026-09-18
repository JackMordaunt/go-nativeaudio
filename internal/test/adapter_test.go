package test

import (
	"bytes"
	"testing"

	"git.sr.ht/~jackmordaunt/nativeaudio/pcm"
)

// TestAdapterOverRealAudio streams a real decode through the float64
// adapter, which is how the composable audio libraries consume it.
func TestAdapterOverRealAudio(t *testing.T) {
	d := newDecoder(t)

	buffered, format, err := d.Decode(compressed)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	wantFrames := len(buffered) / (format.Channels * format.BytesPerSample)

	// Once over the buffered output, and once over the incremental
	// stream, which also exercises Close delegating to decoder state.
	stream, err := d.Stream(compressed)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	for _, tc := range []struct {
		name     string
		streamer *pcm.Streamer
	}{
		{"buffered", pcm.NewStreamer(bytes.NewReader(buffered), format)},
		{"streaming", pcm.NewStreamer(stream, format)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.streamer.Close()

			var (
				frames int
				buf    = make([][2]float64, 512)
				peak   float64
			)
			for {
				n, ok := tc.streamer.Stream(buf)
				for _, f := range buf[:n] {
					for _, v := range f {
						if v > peak {
							peak = v
						}
						if v < -1 || v > 1 {
							t.Fatalf("sample %v out of range", v)
						}
					}
				}
				frames += n
				if !ok {
					break
				}
			}
			if err := tc.streamer.Err(); err != nil {
				t.Fatalf("Err: %v", err)
			}
			if frames != wantFrames {
				t.Errorf("got %d frames, want %d", frames, wantFrames)
			}
			// The fixture is music, so it must not be silence.
			if peak <= 0 {
				t.Error("decoded audio is entirely silent or negative")
			}
			t.Logf("%d frames, peak %.4f", frames, peak)
		})
	}
}
