# nativeaudio

`nativeaudio` is a convenience package that provides a Go interface over
native audio APIs.

Output is PCM s16le, playable directly. Stacks that take an `io.Reader`
of s16le, such as oto and Ebitengine, need no adapter. For the ones that
want float64 sample pairs, such as beep, the `pcm` subpackage converts.

This package was inspired by the utter lack of audio decoders for AAC, 
which it turns out is a closed codec that Windows and macOS have bought
licenses for. 

Windows is implemented via the Media Foundation and macOS is implemented
on AudioToolbox/AVFoundation.

Other platforms including Linux shell out to FFmpeg.

FFmpeg's libraries are LGPL, which Go's static linking does not sit well
with, and running it as a sub-process keeps both the licence question and
the patent question with whoever installed it. We take the performance hit
for that.


`go get git.sr.ht/~jackmordaunt/nativeaudio`

```go
package main

import (
	"log"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

func main() {
	d, err := nativeaudio.New()
	if err != nil {
		log.Fatal(err)
	}
	defer d.Close()

	pcm, format, err := d.DecodeFile("audio.m4a")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%d bytes of PCM, %+v", len(pcm), format)
}
```

Or, to just hear it:

```go
import "git.sr.ht/~jackmordaunt/nativeaudio/play"

play.File("audio.m4a")
```

## API

- `New()` returns a `Decoder` holding the platform state. Close it when you
  are done. Independent parts of a program can each hold their own, and
  closing one does not disturb another.
- `Decoder.DecodeFile(path)` and `Decoder.Decode(data)` return s16le PCM
  and a `Format`. The core package has no audio-output dependency.
- `Decoder.StreamFile(path)` and `Decoder.Stream(data)` return a `Stream`,
  an `io.Reader` over the same PCM, so a long track never has to sit in
  memory whole. `Format` is known before the first read. Close it when
  done. Windows decodes incrementally and the ffmpeg backend pipes; macOS
  currently decodes up front and serves from memory.
- `Format` reports `SampleRate`, `Channels` and `BytesPerSample`, which is
  always 2.
- A `Decoder` is safe for concurrent use, and `Close` waits for decodes
  already in flight.
- `New(WithLimits(...))` bounds how long a decode may run and how much PCM
  it may produce. Malformed audio can otherwise make a native decoder
  grind indefinitely, so the default budget is finite. Untrusted input
  should set its own.
- `FFmpegLoad`, `FFmpegDecode` and `FFmpegStream` shell out to ffmpeg
  regardless of platform, and need no `Decoder`.
- The `pcm` subpackage adapts the output to other audio stacks.
  `pcm.NewStreamer` presents s16le as pairs of float64 samples, which
  satisfies beep's `Streamer` and `StreamCloser` without importing beep,
  since Go interfaces are structural. It takes an `io.Reader`, so it works
  over a `Stream` or over buffered PCM.
- The `play` subpackage plays PCM through oto. The core package builds
  for every target Go supports; `play` is bounded by its audio backend,
  which needs cgo on Linux and has no FreeBSD support. Its
  context is fixed to the first file's sample rate and channel count for
  the life of the process.

## Versions

`v1.1.0` is the first release to use. It replaced the package-level
`Start`, `End`, `Load`, `Decode` and `Play` with a `Decoder` value and the
`play` subpackage, renamed `Format.BitDepth` to `BytesPerSample`, made the
Media Foundation bindings internal, and added streaming decodes, bounded
decodes and the `pcm` adapters.

`v1.0.0` is retracted. It went out before the API had settled, and it
decoded 24-bit sources to 24-bit rather than the s16le documented here.
Nothing depended on it.

## Format support

Measured by `TestFormatSupport`, which generates a file per format with
ffmpeg and decodes it. CI runs it on each platform and publishes the
result, so this table comes from machines rather than from vendor
documentation.

| Format | Windows | macOS | Linux and others |
|---|---|---|---|
| AAC-LC in MP4 | yes | yes | yes |
| AAC-LC in ADTS | yes | yes | yes |
| MP3 | yes | yes | yes |
| WAV, 16-bit | yes | yes | yes |
| WAV, 24-bit | yes | yes | yes |
| FLAC | yes | yes | yes |
| ALAC in MP4 | yes | yes | yes |
| Opus in Ogg | yes | yes | yes |
| Vorbis in Ogg | yes | not measured | yes |
| WMA v2 | yes | no | yes |

Both native backends decode far more than the AAC this package was
written for, so it is useful well beyond that. The only gap either of
them has is WMA on macOS, which AudioToolbox rejects outright rather
than mangling. Vorbis on macOS is unmeasured because the ffmpeg build
there could not produce a file to try.

Output is s16le whatever goes in, including from a 24-bit source.
Decoded lengths differ slightly between backends for codecs that carry
priming samples, which is why the tests compare against a per-backend
reference rather than one golden file.

### AAC profiles

These deserve separate billing, because a decoder that handles only the
base profile does not fail on the others. High efficiency AAC is an
AAC-LC core plus an extension that older decoders are meant to ignore,
and ignoring it yields audio at half the sample rate with the treble
missing and no error raised. A backend that supports only AAC-LC is
therefore worse than one that rejects the file.

This table is from vendor documentation rather than measured, because
generating the high efficiency profiles needs an encoder ffmpeg does not
ship by default.

| Profile | Windows | macOS | ffmpeg |
|---|---|---|---|
| AAC-LC | yes, multichannel | yes | yes |
| HE-AAC v1 | yes, multichannel | yes | yes |
| HE-AAC v2 | yes, stereo | yes | yes |
| xHE-AAC | yes, Windows 11 | yes | partial |
| AAC-LD and AAC-ELD | no | yes | yes |

## TODO 

- [ ] macOS: decode incrementally rather than buffering behind `Stream`
- [ ] Linux: something better then shelling out to FFmpeg
