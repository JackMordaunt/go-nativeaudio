package nativeaudio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"
)

// Stream decodes audio incrementally, yielding signed 16-bit
// little-endian PCM through [io.Reader].
//
// It exists because the buffered API holds the whole decode in memory,
// and PCM is roughly an order of magnitude larger than the compressed
// input it came from. A few minutes of stereo audio is tens of
// megabytes, which is fine for a sound effect and wasteful for a track.
//
// A Stream must be closed. Closing the Decoder that produced it waits
// for every open Stream to be closed first.
type Stream struct {
	r      io.ReadCloser
	format Format
	done   func()
}

// Format describes the PCM this Stream produces. It is known as soon as
// the Stream is opened, before any audio is read.
func (s *Stream) Format() Format {
	return s.format
}

// Read fills p with decoded PCM, returning [io.EOF] when the audio is
// exhausted.
func (s *Stream) Read(p []byte) (int, error) {
	return s.r.Read(p)
}

// Close releases the decoder state behind the Stream. It is safe to
// call more than once, and abandoning a Stream before [io.EOF] is fine
// as long as it is closed.
func (s *Stream) Close() error {
	err := s.r.Close()
	if s.done != nil {
		s.done()
		s.done = nil
	}
	return err
}

// newBufferedStream serves PCM that has already been fully decoded.
//
// Backends that cannot yet decode incrementally use this so that the
// streaming API behaves correctly everywhere, at the cost of the memory
// saving on those platforms.
func newBufferedStream(pcm []byte, format Format) *Stream {
	return &Stream{
		r:      io.NopCloser(bytes.NewReader(pcm)),
		format: format,
	}
}

// Limits bound a single decode. A zero field means no limit.
//
// Malformed audio can make a decoder grind: fuzzing found a half-megabyte
// file that Media Foundation decodes indefinitely, a few hundred
// kilobytes of PCM at a time, never finishing and never failing. No
// per-read timeout catches that, because every individual read completes.
// Limits are the backstop, and matter whenever the input is untrusted.
type Limits struct {
	// MaxBytes caps the PCM a decode may produce.
	MaxBytes int64

	// MaxDuration caps how long a decode may run.
	MaxDuration time.Duration
}

// DefaultLimits is applied to a Decoder created without WithLimits.
//
// The duration is generous by orders of magnitude: a healthy backend
// decodes half an hour of audio in well under a second, so anything
// still running after this is not making meaningful progress. No byte
// limit is imposed by default, since a legitimately long recording is
// legitimately large.
func DefaultLimits() Limits {
	return Limits{MaxDuration: 2 * time.Minute}
}

// ErrLimitExceeded reports that a decode hit its [Limits].
var ErrLimitExceeded = errors.New("nativeaudio: decode exceeded its limits")

// limited enforces Limits over a stream.
type limited struct {
	r        io.ReadCloser
	limits   Limits
	read     int64
	deadline time.Time
}

func newLimited(r io.ReadCloser, l Limits) io.ReadCloser {
	if l.MaxBytes <= 0 && l.MaxDuration <= 0 {
		return r
	}
	lr := &limited{r: r, limits: l}
	if l.MaxDuration > 0 {
		lr.deadline = time.Now().Add(l.MaxDuration)
		// Hand the deadline to the backend too, so a read already
		// waiting on the decoder gives up with everything else rather
		// than running on to its own backstop.
		if d, ok := r.(interface{ SetDeadline(time.Time) }); ok {
			d.SetDeadline(lr.deadline)
		}
	}
	return lr
}

func (l *limited) Read(p []byte) (int, error) {
	if !l.deadline.IsZero() && time.Now().After(l.deadline) {
		return 0, fmt.Errorf("%w: still decoding after %s", ErrLimitExceeded, l.limits.MaxDuration)
	}
	if l.limits.MaxBytes > 0 {
		if remaining := l.limits.MaxBytes - l.read; remaining <= 0 {
			return 0, fmt.Errorf("%w: produced more than %d bytes", ErrLimitExceeded, l.limits.MaxBytes)
		} else if int64(len(p)) > remaining {
			p = p[:remaining]
		}
	}
	n, err := l.r.Read(p)
	l.read += int64(n)
	return n, err
}

func (l *limited) Close() error {
	return l.r.Close()
}
