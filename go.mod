module git.sr.ht/~jackmordaunt/nativeaudio

go 1.21

require (
	github.com/ebitengine/oto/v3 v3.2.0
	golang.org/x/sys v0.19.0
)

require github.com/ebitengine/purego v0.7.1 // indirect

// v1.0.0 went out before the API had settled. It still exposed the
// package-level Start, End, Load, Decode and Play that v1.1.0 replaces
// with a Decoder value, and it decoded 24-bit sources to 24-bit rather
// than the s16le the package documents. Nothing depended on it.
retract v1.0.0
