// This file is a temporary audio implementation for macOS and Linux
// platforms. For Linux I suspect ffmpeg will be the defacto, but macOS
// ships an AAC decoder that we can access directly. That is preferable
// to relying on the ffmpeg binary being present since we'd have to
// provide one or hope for the best.

package nativeaudio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// FFmpegLoad raw PCM with ffmpeg.
//
//	ffmpeg -i <path> -f s16le -
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

// FFmpegDecode raw PCM with ffmpeg.
//
//	ffmpeg -f m4a -i pipe: -f s16le -
//
// s16le is the PCM format specifier, the final dash means "pipe to
// stdout".
//
// NOTE(jfm): unfortunately, some formats cannot be piped, so we will
// create a temporary file instead.
func FFmpegDecode(by []byte) ([]byte, Format, error) {
	// CreateTemp picks a unique name, so concurrent decodes in this or any
	// other process cannot collide.
	tmp, err := os.CreateTemp("", "nativeaudio-*")
	if err != nil {
		return nil, Format{}, fmt.Errorf("creating tmp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(by); err != nil {
		tmp.Close()
		return nil, Format{}, fmt.Errorf("writing tmp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, Format{}, fmt.Errorf("closing tmp file: %w", err)
	}
	return FFmpegLoad(tmp.Name())
}

// probe queries the format information for a given audio file by parsing
// ffprobe results.
//
//	ffprobe -i <path> -v quiet -print_format json -show_format -show_streams
func probe(path string) (Format, error) {
	stdout := bytes.NewBuffer(nil)
	cmd := exec.Command(
		"ffprobe",
		"-i", path,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
	)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Run(); err != nil {
		return Format{}, fmt.Errorf("%q: %w %s", strings.Join(cmd.Args, " "), err, stdout.String())
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
		BytesPerSample: 2,
	}, nil
}

// ffmpegStream pipes PCM out of a running ffmpeg process.
type ffmpegStream struct {
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	stderr   *bytes.Buffer
	waitOnce sync.Once
	waitErr  error
	drained  bool
}

// reap waits for ffmpeg exactly once, whoever gets there first.
func (f *ffmpegStream) reap() error {
	f.waitOnce.Do(func() { f.waitErr = f.cmd.Wait() })
	return f.waitErr
}

func (f *ffmpegStream) Read(p []byte) (int, error) {
	n, err := f.stdout.Read(p)
	if errors.Is(err, io.EOF) {
		// The pipe closing means ffmpeg is finished, so collect its exit
		// status: a decode that failed halfway still reaches EOF here.
		f.drained = true
		if werr := f.reap(); werr != nil {
			return n, fmt.Errorf("ffmpeg: %w: %s", werr, f.stderr.String())
		}
	}
	return n, err
}

func (f *ffmpegStream) Close() error {
	if !f.drained {
		// Abandoned early. Kill it rather than leave ffmpeg blocked
		// writing into a pipe nobody is reading, then reap the corpse.
		// The resulting wait error is ours, not a decode failure.
		_ = f.cmd.Process.Kill()
		_ = f.reap()
		return nil
	}
	return f.reap()
}

// FFmpegStream decodes an audio file with ffmpeg, returning PCM through
// an [io.Reader] as ffmpeg produces it.
//
//	ffmpeg -i <path> -f s16le -
//
// Unlike FFmpegLoad this never holds the whole decode in memory. The
// returned Stream must be closed, including when abandoned early.
func FFmpegStream(path string) (*Stream, error) {
	format, err := probe(path)
	if err != nil {
		return nil, fmt.Errorf("probing file for metadata: %w", err)
	}
	cmd := exec.Command(
		"ffmpeg",
		"-i", path,
		"-f", "s16le",
		"-",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("attaching to ffmpeg output: %w", err)
	}
	stderr := bytes.NewBuffer(nil)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting ffmpeg: %w", err)
	}
	return &Stream{
		r:      &ffmpegStream{cmd: cmd, stdout: stdout, stderr: stderr},
		format: format,
	}, nil
}
