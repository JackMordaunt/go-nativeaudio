//go:build linux && cgo

package nativeaudio

// play stub for Linux.
func play(path string) error {
	return ffmpegPlay(path)
}

// load stub for Linux.
func load(path string) ([]byte, error) {
	return ffmpegLoad(path)
}
