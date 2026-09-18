package nativeaudio

import (
	"fmt"
	"os"

	"git.sr.ht/~jackmordaunt/nativeaudio/internal/mf"
)

func start() error {
	return mf.Startup()
}

func end() error {
	return mf.Shutdown()
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
