package nativeaudio

import (
	"bytes"
	"io"
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
