package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

// formatProbe is one row of the support matrix: a way to produce a file
// with ffmpeg, and whether the package promises to decode it.
type formatProbe struct {
	// name labels the row in the matrix.
	name string
	// file is the name to write, whose extension picks the container.
	file string
	// args are the ffmpeg codec arguments.
	args []string
	// guaranteed marks formats the readme claims support for. Those are
	// assertions; everything else is reported and left to the matrix.
	guaranteed bool
}

var formatProbes = []formatProbe{
	{"AAC-LC in MP4", "out.m4a", []string{"-c:a", "aac", "-b:a", "128k"}, true},
	{"AAC-LC in ADTS", "out.aac", []string{"-c:a", "aac", "-b:a", "128k", "-f", "adts"}, true},
	{"MP3", "out.mp3", []string{"-c:a", "libmp3lame"}, true},
	{"WAV 16-bit", "out.wav", []string{"-c:a", "pcm_s16le"}, true},
	{"WAV 24-bit", "out24.wav", []string{"-c:a", "pcm_s24le"}, true},
	{"FLAC", "out.flac", []string{"-c:a", "flac"}, false},
	{"ALAC in MP4", "alac.m4a", []string{"-c:a", "alac"}, false},
	{"Opus in Ogg", "out.opus", []string{"-c:a", "libopus"}, false},
	{"Vorbis in Ogg", "out.ogg", []string{"-c:a", "libvorbis"}, false},
	{"WMA v2", "out.wma", []string{"-c:a", "wmav2"}, false},
}

// generate writes one second of tone in the probe's format and returns
// its path, or skips when ffmpeg cannot produce it.
func (p formatProbe) generate(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, p.file)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "1", "-ac", "2",
	}
	args = append(args, p.args...)
	args = append(args, path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot produce %s: %v: %s", p.name, err, out)
	}
	return path
}

// TestFormatSupport decodes one file per format and reports what the
// platform's decoder made of it.
//
// The published support matrix comes from running this on each platform,
// which is the only way to keep it honest: the operating system decoders
// handle far more than AAC, and which extras they handle differs. Rows
// marked guaranteed are assertions; the rest are reported so the matrix
// can be filled in from a CI run rather than from vendor documentation.
func TestFormatSupport(t *testing.T) {
	requireTools(t, "ffmpeg")
	dir := t.TempDir()

	for _, probe := range formatProbes {
		t.Run(probe.name, func(t *testing.T) {
			path := probe.generate(t, dir)

			in, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading generated file: %v", err)
			}

			d := newDecoder(t)
			pcm, format, err := d.Decode(in)
			if err != nil {
				if probe.guaranteed {
					t.Errorf("MATRIX %-16s UNSUPPORTED (but claimed): %v", probe.name, err)
				} else {
					t.Logf("MATRIX %-16s unsupported: %v", probe.name, err)
				}
				return
			}

			t.Logf("MATRIX %-16s native supported: %d bytes, %+v", probe.name, len(pcm), format)

			// The ffmpeg fallback is the backend on platforms with no
			// native decoder, so probe it wherever ffmpeg exists rather
			// than only where it is in use.
			if fpcm, fformat, ferr := nativeaudio.FFmpegDecode(in); ferr != nil {
				t.Logf("MATRIX %-16s ffmpeg unsupported: %v", probe.name, ferr)
			} else {
				t.Logf("MATRIX %-16s ffmpeg supported: %d bytes, %+v", probe.name, len(fpcm), fformat)
			}

			// Whatever the input, the output contract is the same.
			if format.BytesPerSample != 2 {
				t.Errorf("%s decoded to %d bytes per sample, want 2", probe.name, format.BytesPerSample)
			}
			if format.SampleRate < 1 || format.Channels < 1 {
				t.Errorf("%s reported an implausible format: %+v", probe.name, format)
			}
			if len(pcm) == 0 {
				t.Errorf("%s decoded to no audio", probe.name)
			}
		})
	}
}
