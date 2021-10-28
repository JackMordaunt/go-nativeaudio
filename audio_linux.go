//go:build linux && cgo

package nativeaudio

// play stub for Linux.
func play(path string) error {
	return FFmpegPlay(path)
}

// load stub for Linux.
func load(path string) ([]byte, Format, error) {
	return FFmpegLoad(path)
}

// decode stub for Linux.
func decode(by []byte) ([]byte, Format, error) {
	return FFmpegDecode(by)
}

func start() error {
	return nil
}

func end() error {
	return nil
}
