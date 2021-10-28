//go:build !(windows || darwin || linux) || !cgo

// Shell out to ffmpeg if either the OS is unknown or cgo is disabled.

package nativeaudio

// play audio with ffmpeg.
func play(path string) error {
	return FFmpegPlay(path)
}

// load and decode an audio file with ffmpeg.
func load(path string) ([]byte, Format, error) {
	return FFmpegLoad(path)
}

// decode a bytes with ffmpeg.
func decode(by []byte) ([]byte, Format, error) {
	return FFmpegDecode(by)
}

// start stub.
func start() error {
	return nil
}

// end stub.
func end() error {
	return nil
}
