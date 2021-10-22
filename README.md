# nativeaudio

`nativeaudio` is a convenience package that provides a Go interface over
native audio APIs.

The resultant output should be PCM s16le playable directly and also by
github.com/hajimehoshi/oto.

This package was inspired by the utter lack of audio decoders for AAC, 
which it turns out is a closed codec that Windows and macOS have bought
licenses for. 

For the moment Windows is implemented via the Media Foundation, and 
other platforms shell out to FFmpeg as a fallback. 

macOS bindings are expected to land relatively soon, however the other
platforms are left as an exercise to the community. 

`go get git.sr.ht/~jackmordaunt/nativeaudio`

```go
package main 

import "git.sr.ht/~jackmordaunt/nativeaudio"

func main() {
        nativeaudio.Play("audio.m4a")
}
```

## TODO 

- [ ] report back the sample rate, number of channels and bit depth 
- [ ] macOS native bindings 
- [ ] byte level `Decode([]byte) []byte` function that avoids file system dependency all together 
- [ ] optimize memory usage  
        - we can manage memory entirely from the Go side with a touch more orchestration
