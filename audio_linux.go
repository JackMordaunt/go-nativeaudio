//go:build linux && cgo

package nativeaudio

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
