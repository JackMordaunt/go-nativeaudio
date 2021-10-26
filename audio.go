// Package nativeaudio leverages native decoders for each supported OS
// to decode raw PCM data.
//
// Where there are native APIs to call we default to invoking ffmpeg.
//
// 	Windows: Media Foundation
// 	  macOS: ffmepg (pending native bindings)
// 	  Linux: ffmpeg
//
package nativeaudio

// Play an audio file exactly once, synchronously.
func Play(path string) error {
	return play(path)
}

// Load compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func Load(path string) (uncompressed []byte, err error) {
	return load(path)
}

// Decode compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func Decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	return decode(compressed)
}

// Format desribes the features of the associated PCM data necessary
// for correct playback.
type Format struct {
	SampleRate int
	Channels   int
	BitDepth   int
}
