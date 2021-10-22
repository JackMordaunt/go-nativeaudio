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

// Load decode and buffer an audio file. The buffer should contain raw
// PCM data (s16le).
func Load(path string) (pcm []byte, err error) {
	return load(path)
}
