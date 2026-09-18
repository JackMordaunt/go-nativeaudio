// Package nativeaudio leverages native decoders for each supported OS
// to decode raw PCM data.
//
// Where there are no native APIs to call we default to invoking ffmpeg.
//
//	Windows: Media Foundation
//	  macOS: AudioToolbox
//	  Linux: ffmpeg
package nativeaudio

import (
	"sync"
)

var (
	mu          sync.Mutex
	initialized bool = false
)

// Start initializes any platform code required.
func Start() error {
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}
	if err := start(); err != nil {
		return err
	}
	initialized = true
	return nil
}

// End cleans up platform code, if any.
func End() error {
	mu.Lock()
	defer mu.Unlock()
	if !initialized {
		return nil
	}
	if err := end(); err != nil {
		return err
	}
	initialized = false
	return nil
}

// Load compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func Load(path string) (uncompressed []byte, format Format, err error) {
	if err := Start(); err != nil {
		return nil, format, err
	}
	return load(path)
}

// Decode compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func Decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	if err := Start(); err != nil {
		return nil, format, err
	}
	return decode(compressed)
}

// Format describes the features of the associated PCM data necessary
// for correct playback.
type Format struct {
	SampleRate     int // samples per second.
	Channels       int // number channels.
	BytesPerSample int // bytes per sample; 2 for the s16le output this package produces.
}
