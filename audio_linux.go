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

// openStream decodes up front, because ffmpeg cannot read compressed
// audio from a pipe for every container we support.
func openStream(by []byte) (*Stream, error) {
	pcm, format, err := FFmpegDecode(by)
	if err != nil {
		return nil, err
	}
	return newBufferedStream(pcm, format), nil
}

// openStreamFile pipes PCM straight out of ffmpeg.
func openStreamFile(path string) (*Stream, error) {
	return FFmpegStream(path)
}
