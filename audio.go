// Package nativeaudio decodes compressed audio into PCM using the
// decoder each operating system already ships, falling back to ffmpeg
// where there is no native API to call.
//
//	Windows: Media Foundation
//	  macOS: AudioToolbox
//	  Linux: ffmpeg
//
// Output is always signed 16-bit little-endian PCM, which is directly
// playable and is what the common Go audio stacks expect. The play
// subpackage is a thin convenience over that for callers who just want
// to hear a file.
package nativeaudio

import (
	"errors"
	"fmt"
	"sync"
)

// ErrClosed is returned when a Decoder is used after Close.
var ErrClosed = errors.New("nativeaudio: decoder is closed")

// Decoder decodes compressed audio into PCM.
//
// Create one with New and release it with Close. A Decoder owns
// whatever platform state the backend requires, which is why it is a
// value rather than a set of package functions: two independent parts
// of a program can hold their own without one tearing down the other.
//
// A Decoder is safe for concurrent use. Decodes may run in parallel,
// and Close waits for those in flight to finish.
type Decoder struct {
	mu     sync.RWMutex
	closed bool
}

// New creates a Decoder, initialising any platform state the backend
// needs. Call Close when you are finished with it.
func New() (*Decoder, error) {
	if err := start(); err != nil {
		return nil, fmt.Errorf("initialising platform decoder: %w", err)
	}
	return &Decoder{}, nil
}

// Close releases the platform state held by the Decoder. It is
// idempotent, and any further use of the Decoder returns ErrClosed.
func (d *Decoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	if err := end(); err != nil {
		return fmt.Errorf("shutting down platform decoder: %w", err)
	}
	return nil
}

// DecodeFile decodes the audio file at path, returning s16le PCM and
// the format needed to play it back correctly.
func (d *Decoder) DecodeFile(path string) (pcm []byte, format Format, err error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, format, ErrClosed
	}
	return load(path)
}

// Decode decodes compressed audio held in memory, returning s16le PCM
// and the format needed to play it back correctly.
func (d *Decoder) Decode(compressed []byte) (pcm []byte, format Format, err error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, format, ErrClosed
	}
	return decode(compressed)
}

// Format describes the PCM a decode produced, and is everything needed
// to play it back correctly.
type Format struct {
	SampleRate     int // samples per second.
	Channels       int // number of channels.
	BytesPerSample int // bytes per sample; always 2, for the s16le output this package produces.
}
