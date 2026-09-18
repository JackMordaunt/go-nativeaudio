package pcm_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"git.sr.ht/~jackmordaunt/nativeaudio"
	"git.sr.ht/~jackmordaunt/nativeaudio/pcm"
)

// The interfaces beep defines, restated here. Asserting against a local
// copy pins the signatures the adapter has to match without taking on the
// dependency, so a change to either side shows up as a build failure.
type (
	beepStreamer interface {
		Stream(samples [][2]float64) (n int, ok bool)
		Err() error
	}
	beepStreamCloser interface {
		beepStreamer
		Close() error
	}
)

var _ beepStreamCloser = (*pcm.Streamer)(nil)

// s16 encodes samples as little-endian 16-bit PCM.
func s16(samples ...int16) []byte {
	var b bytes.Buffer
	for _, s := range samples {
		binary.Write(&b, binary.LittleEndian, s)
	}
	return b.Bytes()
}

func stereo(rate int) nativeaudio.Format {
	return nativeaudio.Format{SampleRate: rate, Channels: 2, BytesPerSample: 2}
}

func mono(rate int) nativeaudio.Format {
	return nativeaudio.Format{SampleRate: rate, Channels: 1, BytesPerSample: 2}
}

// near reports whether two samples match within rounding.
func near(t *testing.T, a, b float64) bool {
	t.Helper()
	return math.Abs(a-b) < 1e-9
}

// TestStereoConversion checks the sample scaling and channel order.
func TestStereoConversion(t *testing.T) {
	in := s16(0, 32767, -32768, 16384)
	s := pcm.NewStreamer(bytes.NewReader(in), stereo(44100))

	got := make([][2]float64, 4)
	n, ok := s.Stream(got)
	if !ok || n != 2 {
		t.Fatalf("Stream: got n=%d ok=%v, want 2 frames", n, ok)
	}
	if err := s.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}

	want := [][2]float64{
		{0, 32767.0 / 32768.0},
		{-1, 0.5},
	}
	for i := range want {
		if !near(t, got[i][0], want[i][0]) || !near(t, got[i][1], want[i][1]) {
			t.Errorf("frame %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

// TestMonoIsDuplicated checks that one channel reaches both ears rather
// than being silently dropped or halving the frame count.
func TestMonoIsDuplicated(t *testing.T) {
	in := s16(16384, -16384)
	s := pcm.NewStreamer(bytes.NewReader(in), mono(22050))

	got := make([][2]float64, 4)
	n, ok := s.Stream(got)
	if !ok || n != 2 {
		t.Fatalf("Stream: got n=%d ok=%v, want 2 frames", n, ok)
	}
	for i, want := range []float64{0.5, -0.5} {
		if !near(t, got[i][0], want) || !near(t, got[i][1], want) {
			t.Errorf("frame %d: got %v, want both channels %v", i, got[i], want)
		}
	}
}

// dribble returns at most n bytes per Read, so frames land split across
// calls. The underlying decoder is free to do this, and the earlier
// small-read test showed it does.
type dribble struct {
	r io.Reader
	n int
}

func (d dribble) Read(p []byte) (int, error) {
	if len(p) > d.n {
		p = p[:d.n]
	}
	return d.r.Read(p)
}

// TestPartialFramesAcrossReads streams through a reader that splits
// frames, and checks nothing is lost or misaligned.
func TestPartialFramesAcrossReads(t *testing.T) {
	const frames = 100
	samples := make([]int16, 0, frames*2)
	for i := 0; i < frames; i++ {
		samples = append(samples, int16(i), int16(-i))
	}
	in := s16(samples...)

	// Three bytes at a time never aligns with a four byte frame.
	s := pcm.NewStreamer(dribble{r: bytes.NewReader(in), n: 3}, stereo(44100))

	var got [][2]float64
	buf := make([][2]float64, 7)
	for {
		n, ok := s.Stream(buf)
		got = append(got, buf[:n]...)
		if !ok {
			break
		}
	}
	if err := s.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	if len(got) != frames {
		t.Fatalf("got %d frames, want %d", len(got), frames)
	}
	for i := range got {
		wantL := float64(int16(i)) / 32768
		wantR := float64(int16(-i)) / 32768
		if !near(t, got[i][0], wantL) || !near(t, got[i][1], wantR) {
			t.Fatalf("frame %d: got %v, want {%v %v}", i, got[i], wantL, wantR)
		}
	}
}

// TestRejectsUnsupportedFormat ensures a format the adapter cannot honour
// is reported rather than producing quiet nonsense.
func TestRejectsUnsupportedFormat(t *testing.T) {
	s := pcm.NewStreamer(bytes.NewReader(s16(1, 2, 3, 4)), nativeaudio.Format{
		SampleRate: 44100, Channels: 2, BytesPerSample: 3,
	})
	if n, ok := s.Stream(make([][2]float64, 4)); ok || n != 0 {
		t.Fatalf("Stream: got n=%d ok=%v, want it to refuse", n, ok)
	}
	if s.Err() == nil {
		t.Fatal("Err: want an error for a 3 byte sample size")
	}
}

// closeSpy records whether the adapter closed what it was given.
type closeSpy struct {
	io.Reader
	closed bool
}

func (c *closeSpy) Close() error {
	c.closed = true
	return nil
}

// TestCloseDelegates covers the streaming case, where the reader owns
// decoder state that has to be released.
func TestCloseDelegates(t *testing.T) {
	spy := &closeSpy{Reader: bytes.NewReader(s16(1, 2))}
	s := pcm.NewStreamer(spy, stereo(44100))
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !spy.closed {
		t.Error("Close did not close the underlying reader")
	}

	// A plain reader is not a closer, and that must not be an error.
	if err := pcm.NewStreamer(bytes.NewReader(nil), stereo(44100)).Close(); err != nil {
		t.Errorf("Close on a non-closer: %v", err)
	}
}

// TestEmptyInput checks the degenerate case reports no frames rather than
// blocking or panicking.
func TestEmptyInput(t *testing.T) {
	s := pcm.NewStreamer(bytes.NewReader(nil), stereo(44100))
	if n, ok := s.Stream(make([][2]float64, 4)); ok || n != 0 {
		t.Fatalf("Stream: got n=%d ok=%v, want no frames", n, ok)
	}
	if err := s.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
}
