//go:build darwin && cgo

package nativeaudio

// play stub for macOS.
func play(path string) error {
	return FFmpegPlay(path)
}

// load stub for macOS.
func load(path string) ([]byte, Format, error) {
	return FFmpegLoad(path)
}

// decode stub for macOS.
func decode(by []byte) ([]byte, Format, error) {
	return FFmpegDecode(by)
}
