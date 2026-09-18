package nativeaudio

import (
	"fmt"
	"io"
	"os"

	"git.sr.ht/~jackmordaunt/nativeaudio/internal/mf"
)

func start() error {
	return mf.Startup()
}

func end() error {
	return mf.Shutdown()
}

// load raw pcm data from the Windows Media Foundation.
func load(path string) (uncompressed []byte, format Format, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, format, fmt.Errorf("opening input file: %w", err)
	}
	defer f.Close()
	by, err := io.ReadAll(f)
	if err != nil {
		return nil, format, fmt.Errorf("buffering input file: %w", err)
	}
	return decode(by)
}

// decode compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	pcm, f, err := mf.Decode(compressed)
	if err != nil {
		return nil, format, err
	}
	return pcm, Format{
		SampleRate:     f.SampleRate,
		Channels:       f.Channels,
		BytesPerSample: f.BytesPerSample,
	}, nil
}

// openStream decodes incrementally through Media Foundation.
func openStream(compressed []byte) (*Stream, error) {
	ms, err := mf.Open(compressed)
	if err != nil {
		return nil, err
	}
	f := ms.Format()
	return &Stream{
		r: ms,
		format: Format{
			SampleRate:     f.SampleRate,
			Channels:       f.Channels,
			BytesPerSample: f.BytesPerSample,
		},
	}, nil
}

// openStreamFile buffers the compressed file, which is small next to
// the PCM the stream avoids holding, then decodes it incrementally.
func openStreamFile(path string) (*Stream, error) {
	by, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading input file: %w", err)
	}
	return openStream(by)
}
