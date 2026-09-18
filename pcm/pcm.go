// Package pcm adapts the PCM this module produces to the shapes other Go
// audio libraries consume.
//
// Stacks that take an [io.Reader] of signed 16-bit little-endian samples,
// such as oto and Ebitengine's audio package, need no adapter: a
// [nativeaudio.Stream] is already one, and buffered output can be wrapped
// in a [bytes.Reader].
//
// Libraries built around composable streams want something else. They
// deal in pairs of float64 samples, so [Streamer] converts. It satisfies
// beep's Streamer and StreamCloser interfaces without importing beep,
// because Go interfaces are structural and neither side needs to know
// about the other.
package pcm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

// sampleScale converts a signed 16-bit sample to the range [-1, 1).
const sampleScale = 1 << 15

// Streamer presents s16le PCM as pairs of float64 samples.
//
// Mono input is written to both channels. Input with more than two
// channels keeps the first two, because the interface this serves is
// stereo and mixing down is a decision the caller should make rather
// than inherit.
type Streamer struct {
	r      io.Reader
	format nativeaudio.Format

	// partial holds the bytes of a frame split across two reads, since
	// the underlying reader is free to stop mid-frame.
	partial []byte

	buf  []byte
	err  error
	done bool
}

// NewStreamer adapts s16le PCM read from r.
//
// Pass the [nativeaudio.Format] that came back with the audio. Taking a
// reader rather than a stream means the buffered API works too, through
// a [bytes.Reader] over the decoded PCM.
func NewStreamer(r io.Reader, format nativeaudio.Format) *Streamer {
	return &Streamer{r: r, format: format}
}

// Stream fills samples with as many frames as it can, returning the
// number written and whether streaming should continue.
//
// It reports false once the audio is exhausted or an error has occurred,
// which is the convention the consuming interface expects; the error
// itself comes from [Streamer.Err].
func (s *Streamer) Stream(samples [][2]float64) (int, bool) {
	if s.done || len(samples) == 0 {
		return 0, false
	}
	if s.format.BytesPerSample != 2 {
		s.fail(fmt.Errorf("pcm: unsupported sample size %d bytes, want 2", s.format.BytesPerSample))
		return 0, false
	}
	if s.format.Channels < 1 {
		s.fail(fmt.Errorf("pcm: unsupported channel count %d", s.format.Channels))
		return 0, false
	}

	var (
		frame = s.format.Channels * s.format.BytesPerSample
		want  = len(samples) * frame
	)
	if cap(s.buf) < want {
		s.buf = make([]byte, want)
	}
	s.buf = s.buf[:want]

	// Start from whatever was left over last time.
	n := copy(s.buf, s.partial)
	s.partial = s.partial[:0]

	read, err := io.ReadFull(s.r, s.buf[n:])
	n += read
	switch {
	case err == nil, errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		// A short read is normal: it means the audio ended.
	default:
		s.fail(err)
		if n == 0 {
			return 0, false
		}
	}

	frames := n / frame
	if rest := n % frame; rest != 0 {
		// Hold the trailing bytes for the next call. At the end of the
		// audio they are a truncated frame and stay unplayed, which is
		// the only sane reading of a file that ends mid-frame.
		s.partial = append(s.partial[:0], s.buf[n-rest:n]...)
	}

	for i := 0; i < frames; i++ {
		base := i * frame
		left := sample(s.buf[base:])
		right := left
		if s.format.Channels > 1 {
			right = sample(s.buf[base+2:])
		}
		samples[i] = [2]float64{left, right}
	}

	if frames == 0 {
		s.done = true
		return 0, false
	}
	// The reader is exhausted, so this is the last batch.
	if err != nil {
		s.done = true
	}
	return frames, true
}

// Err returns the first error encountered, if any. Reaching the end of
// the audio is not an error.
func (s *Streamer) Err() error {
	return s.err
}

// Close closes the underlying reader when it is an [io.Closer], which is
// the case for a [nativeaudio.Stream]. Otherwise it does nothing.
func (s *Streamer) Close() error {
	s.done = true
	if c, ok := s.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Format describes the audio being streamed, as sample rate and channel
// count. It is the format passed to [NewStreamer], unchanged.
func (s *Streamer) Format() nativeaudio.Format {
	return s.format
}

// fail records the first error and stops streaming.
func (s *Streamer) fail(err error) {
	if s.err == nil {
		s.err = err
	}
	s.done = true
}

// sample reads one little-endian 16-bit sample as a float in [-1, 1).
func sample(b []byte) float64 {
	return float64(int16(binary.LittleEndian.Uint16(b))) / sampleScale
}
