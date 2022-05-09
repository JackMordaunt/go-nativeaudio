// This file is a temporary audio implementation for macOS and Linux
// platforms. For Linux I suspect ffmpeg will be the defacto, but macOS
// ships an AAC decoder that we can access directly. That is preferable
// to relying on the ffmepg binary being present since we'd have to
// provide one or hope for the best.

package nativeaudio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// FFmpegPlay an audio file with ffplay.
//
// 	ffplay -vn <path> -nodisp -autoexit
//
// -vn: no video,
// -nodisp: do not launch graphical window,
// -autoexit: exit the process after playback is complete.
func FFmpegPlay(path string) error {
	if out, err := exec.Command(
		"ffplay",
		"-vn",
		path,
		"-nodisp",
		"-autoext",
	).CombinedOutput(); err != nil {
		return fmt.Errorf("ffplay: %s: %w", string(out), err)
	}
	return nil
}

// FFmpegLoad raw PCM with ffmpeg.
//
// 	ffmpeg -i <path> -f s16le -
//
// s16le is the PCM format specifier, the final dash means "pipe to
// stdout".
func FFmpegLoad(path string) ([]byte, Format, error) {
	f, err := probe(path)
	if err != nil {
		return nil, f, fmt.Errorf("probing file for metadata: %w", err)
	}
	buffer := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	cmd := exec.Command(
		"ffmpeg",
		"-i", path,
		"-f", "s16le",
		"-",
	)
	cmd.Stdout = buffer
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return nil, Format{}, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return buffer.Bytes(), f, nil
}

var count = counter{}

type counter struct {
	count int
	m     sync.Mutex
}

func (c *counter) Next() int {
	c.m.Lock()
	defer c.m.Unlock()
	c.count++
	return c.count
}

func (c *counter) Done() {
	c.m.Lock()
	defer c.m.Unlock()
	c.count--
}

// FFmpegDecode raw PCM with ffmpeg.
//
// 	ffmpeg -f m4a -i pipe: -f s16le -
//
// s16le is the PCM format specifier, the final dash means "pipe to
// stdout".
//
// NOTE(jfm): unfortunately, some formats cannot be piped, so we will
// create a temporary file instead.
func FFmpegDecode(by []byte) ([]byte, Format, error) {
	// id ensures that multiple concurrent tmp files do not collide.
	id := count.Next()
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("nativeaudio-%d", id))
	if err := os.WriteFile(tmp, by, 0644); err != nil {
		return nil, Format{}, fmt.Errorf("creating tmp file: %w", err)
	}
	defer os.Remove(tmp)
	return FFmpegLoad(tmp)
}

// probe queries the format information for a given audio file by parsing
// ffprobe results.
//
//	ffprobe -i <path> -v quiet -print_format json -show_format -show_streams
//
func probe(path string) (Format, error) {
	var (
		stderr = bytes.NewBuffer(nil)
		stdout = bytes.NewBuffer(nil)
	)
	cmd := exec.Command(
		"ffprobe",
		"-i", path,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
	)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return Format{}, fmt.Errorf("%q: %w %s", strings.Join(cmd.Args, " "), err, stderr.String())
	}
	var f md
	if err := json.Unmarshal(stdout.Bytes(), &f); err != nil {
		return Format{}, fmt.Errorf("unmarshalling json: %w", err)
	}
	s := f.FirstAudioStream()
	if s == nil {
		return Format{}, fmt.Errorf("file has no audio streams")
	}
	return s.Format()
}

// md partially describes the structured metadata output from ffprobe.
type md struct {
	// Streams contains all streams, we will filter for "first audio stream".
	Streams []stream `json:"streams"`
}

// stream description.
type stream struct {
	// CodecType is the "major" type: {audio,video}.
	CodecType string `json:"codec_type"`
	// SampleRate in Hz.
	SampleRate string `json:"sample_rate"`
	// Channel count.
	Channels int `json:"channels"`
}

// FirstAudioStream returns the first audio stream described, if any.
func (f md) FirstAudioStream() *stream {
	for ii, s := range f.Streams {
		if s.CodecType == "audio" {
			return &f.Streams[ii]
		}
	}
	return nil
}

// Format returns the unified Format struct from stream info.
func (s stream) Format() (Format, error) {
	if s.Channels < 1 || s.Channels > 2 {
		return Format{}, fmt.Errorf("can only handle {1,2} channels got %d", s.Channels)
	}
	sr, err := strconv.Atoi(s.SampleRate)
	if err != nil {
		return Format{}, fmt.Errorf("invalid sample rate: must be number got %q", s.SampleRate)
	}
	return Format{
		SampleRate: sr,
		Channels:   s.Channels,
		// We are going to tell ffmpeg to output s16le, though there
		// might be a better place to make this assumption.
		BitDepth: 2,
	}, nil
}
