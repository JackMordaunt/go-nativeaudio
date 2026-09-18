// Package play provides synchronous audio playback for PCM produced by
// the nativeaudio package.
//
// It is deliberately a separate package. Decoding is the reason
// nativeaudio exists, and most callers want PCM to hand to an audio
// stack they have already chosen. Keeping playback here means the core
// package does not drag an output stack into those programs.
package play

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"git.sr.ht/~jackmordaunt/nativeaudio"
	"github.com/ebitengine/oto/v3"
)

var (
	mu        sync.Mutex
	shared    *oto.Context
	sharedFmt nativeaudio.Format
)

// sharedContext returns the process-wide oto context, creating it on
// first use. oto permits exactly one context per process and fixes its
// sample rate and channel count at creation, so every file played after
// the first must share the first file's format.
func sharedContext(f nativeaudio.Format) (*oto.Context, error) {
	mu.Lock()
	defer mu.Unlock()
	if shared != nil {
		if f != sharedFmt {
			return nil, fmt.Errorf("playback context is fixed at %d Hz, %d channel(s) by the first file played; cannot play %d Hz, %d channel(s) in the same process",
				sharedFmt.SampleRate, sharedFmt.Channels, f.SampleRate, f.Channels)
		}
		return shared, nil
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
	shared, sharedFmt = ctx, f
	return ctx, nil
}

// PCM plays signed 16-bit little-endian PCM synchronously, returning
// once the audio has finished playing.
func PCM(pcm []byte, format nativeaudio.Format) error {
	if format.BytesPerSample != 2 {
		return fmt.Errorf("playback: unsupported sample size %d bytes, want 2", format.BytesPerSample)
	}
	ctx, err := sharedContext(format)
	if err != nil {
		return err
	}
	if err := ctx.Resume(); err != nil {
		return fmt.Errorf("resuming playback context: %w", err)
	}
	player := ctx.NewPlayer(bytes.NewReader(pcm))
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

// File decodes an audio file and plays it once, synchronously.
func File(path string) error {
	d, err := nativeaudio.New()
	if err != nil {
		return err
	}
	defer d.Close()
	pcm, format, err := d.DecodeFile(path)
	if err != nil {
		return fmt.Errorf("decoding %q: %w", path, err)
	}
	return PCM(pcm, format)
}

// Data decodes compressed audio and plays it once, synchronously.
func Data(compressed []byte) error {
	d, err := nativeaudio.New()
	if err != nil {
		return err
	}
	defer d.Close()
	pcm, format, err := d.Decode(compressed)
	if err != nil {
		return fmt.Errorf("decoding: %w", err)
	}
	return PCM(pcm, format)
}
