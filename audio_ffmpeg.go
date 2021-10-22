// This file is a temporary audio implementation for macOS and Linux
// platforms. For Linux I suspect ffmpeg will be the defacto, but macOS
// ships an AAC decoder that we can access directly. That is preferable
// to relying on the ffmepg binary being present since we'd have to
// provide one or hope for the best.

package nativeaudio

import (
	"bytes"
	"fmt"
	"os/exec"
)

// play an audio file with ffplay.
//
// 	ffplay -vn <path> -nodisp -autoexit
//
// -vn: no video,
// -nodisp: do not launch graphical window,
// -autoexit: exit the process after playback is complete.
func ffmpegPlay(path string) error {
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

// load raw PCM with ffmpeg.
//
// 	ffmpeg -i <path> -f s16le -
//
// s16le is the PCM format specifier, the final dash means "pipe to
// stdout".
func ffmpegLoad(path string) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	cmd := exec.Command(
		"ffmpeg",
		"-i", path,
		"-f", "s16le",
		"-",
	)
	cmd.Stdout = buffer
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	return buffer.Bytes(), nil
}
