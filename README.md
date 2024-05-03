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

## TODO 

- [ ] streaming API (current API is a buffered for simplicity)
- [ ] Linux: something better then shelling out to FFmpeg
