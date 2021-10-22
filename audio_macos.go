//go:build darwin && cgo

package nativeaudio

// play stub for macOS.
func play(path string) error {
	return ffmpegPlay(path)
}

// load stub for macOS.
func load(path string) ([]byte, error) {
	return ffmpegLoad(path)
}
