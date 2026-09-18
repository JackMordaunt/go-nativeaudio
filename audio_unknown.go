//go:build !windows && (!(darwin || linux) || !cgo)

// Shell out to ffmpeg if either the OS is unknown or cgo is disabled.
// Windows is pure Go and never falls through to this file.

package nativeaudio

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
