//go:build !windows && (!(darwin || linux) || !cgo)

// Shell out to ffmpeg if either the OS is unknown or cgo is disabled.
// Windows is pure Go and never falls through to this file.

package nativeaudio

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
