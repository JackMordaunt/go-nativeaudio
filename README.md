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

import "git.sr.ht/~jackmordaunt/nativeaudio"

func main() {
        nativeaudio.Play("audio.m4a")
}
```

## API

- `Play(path)` decodes a file and plays it synchronously. On Windows and
  macOS the playback context is fixed to the first file's sample rate and
  channel count for the life of the process.
- `Load(path)` and `Decode(data)` return s16le PCM together with a
  `Format` giving `SampleRate`, `Channels` and `BytesPerSample` (always 2).
- `Start()` and `End()` initialise and tear down platform state. The
  functions above call `Start()` for you; call `End()` when you are done.
- `FFmpegPlay`, `FFmpegLoad` and `FFmpegDecode` shell out to ffmpeg
  regardless of platform.

v1.0.0 renamed `Format.BitDepth` to `BytesPerSample` and removed the
Windows Media Foundation bindings from the public API.

## TODO 

- [ ] streaming API (current API is a buffered for simplicity)
- [ ] Linux: something better then shelling out to FFmpeg
