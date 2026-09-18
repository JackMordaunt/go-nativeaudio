# nativeaudio

`nativeaudio` is a convenience package that provides a Go interface over
native audio APIs.

The resultant output should be PCM s16le playable directly and also by
github.com/hajimehoshi/oto and github.com/faiface/beep.

This package was inspired by the utter lack of audio decoders for AAC, 
which it turns out is a closed codec that Windows and macOS have bought
licenses for. 

Windows is implemented via the Media Foundation and macOS is implemented
on AudioToolbox/AVFoundation.

Other platforms including Linux shell out to FFmpeg.

Unfortunately we can't link to FFmpeg without accepting the GPL, so we must
take the performance hit of using a sub-process. 


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
- `Format` reports `SampleRate`, `Channels` and `BytesPerSample`, which is
  always 2.
- A `Decoder` is safe for concurrent use, and `Close` waits for decodes
  already in flight.
- `FFmpegLoad` and `FFmpegDecode` shell out to ffmpeg regardless of
  platform, and need no `Decoder`.
- The `play` subpackage plays PCM through oto on every platform. Its
  context is fixed to the first file's sample rate and channel count for
  the life of the process.

v1.0.0 replaced the package-level `Start`, `End`, `Load`, `Decode` and
`Play` with a `Decoder` value and the `play` subpackage, renamed
`Format.BitDepth` to `BytesPerSample`, and made the Windows Media
Foundation bindings internal.

## TODO 

- [ ] streaming API (current API is a buffered for simplicity)
- [ ] Linux: something better then shelling out to FFmpeg
