//go:build windows || (darwin && cgo)

package nativeaudio

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

var (
	playbackMu     sync.Mutex
	playbackCtx    *oto.Context
	playbackFormat Format
)

// playbackContext returns the process-wide oto context, creating it on
// first use. oto permits exactly one context per process and fixes its
// sample rate and channel count at creation, so every file played after
// the first must share the first file's format.
func playbackContext(f Format) (*oto.Context, error) {
	playbackMu.Lock()
	defer playbackMu.Unlock()
	if playbackCtx != nil {
		if f != playbackFormat {
			return nil, fmt.Errorf("playback context is fixed at %d Hz, %d channel(s) by the first file played; cannot play %d Hz, %d channel(s) in the same process",
				playbackFormat.SampleRate, playbackFormat.Channels, f.SampleRate, f.Channels)
		}
		return playbackCtx, nil
	}
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   f.SampleRate,
		ChannelCount: f.Channels,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, fmt.Errorf("starting playback context: %w", err)
	}
	<-ready
	playbackCtx, playbackFormat = ctx, f
	return ctx, nil
}

// playPCM plays s16le PCM synchronously through the shared oto context,
// returning once the audio has finished.
func playPCM(data []byte, format Format) error {
	if format.BitDepth != 2 {
		return fmt.Errorf("playback: unsupported sample size %d bytes, want 2", format.BitDepth)
	}
	ctx, err := playbackContext(format)
	if err != nil {
		return err
	}
	if err := ctx.Resume(); err != nil {
		return fmt.Errorf("resuming playback context: %w", err)
	}
	player := ctx.NewPlayer(bytes.NewReader(data))
	player.Play()
	// IsPlaying stays true until the source is exhausted AND oto's
	// internal buffer (half a second by default) has been played out, or
	// the player fails. Waiting only for the source to hit EOF would cut
	// off the tail of the audio.
	for player.IsPlaying() {
		time.Sleep(10 * time.Millisecond)
	}
	err = player.Err()
	if cerr := player.Close(); err == nil && cerr != nil {
		err = fmt.Errorf("closing player: %w", cerr)
	}
	if serr := ctx.Suspend(); err == nil && serr != nil {
		err = fmt.Errorf("suspending playback context: %w", serr)
	}
	return err
}
