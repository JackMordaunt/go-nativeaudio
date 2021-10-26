//go:build windows && cgo

package nativeaudio

// -g: add to CFLAGS to include dwarf debug data

/*
#cgo CFLAGS: -Wall -Werror
#cgo LDFLAGS: -lWinmm -lMf -lMfplat  -lMfuuid -loleaut32 -limm32 -lversion -lWindowsApp -lMfreadwrite -lShlwapi
#include "audio_windows.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"
)

// play the audio file using Windows Media Foundation.
func play(path string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	err := C.Play(cPath)
	if err != nil {
		defer C.ErrorFree(err)
		return collectErrors(err)
	}
	return nil
}

// load raw pcm data from the Windows Media Foundation.
//
// PERF(jfm): we can optimize this by allocating the buffer from Go,
// and passing it in for C to fill up. It would require more
// orchestration, but would save the copy. At the moment, C allocates
// its own buffer, we then copy the data and free the C buffer.
func load(path string) (uncompressed []byte, format Format, err error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	r := C.Load(cPath)
	if r.Err != nil {
		defer C.ErrorFree(r.Err)
		return nil, Format{}, collectErrors(r.Err)
	}
	uncompressed = goBytes(unsafe.Pointer(r.Uncompressed.Data), int(r.Uncompressed.Len))
	runtime.SetFinalizer(&uncompressed, func(_ *[]byte) {
		C.BufferFree(r.Uncompressed)
	})
	format = Format{
		SampleRate: int(r.Format.SampleRate),
		BitDepth:   int(r.Format.BitDepth),
		Channels:   int(r.Format.Channels),
	}
	return uncompressed, format, nil
}

// decode compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	defer runtime.KeepAlive(compressed)
	r := C.Decode(cBytes(compressed))
	if r.Err != nil && r.Err.Str != nil {
		defer C.ErrorFree(r.Err)
		return nil, format, collectErrors(r.Err)
	}
	uncompressed = goBytes(unsafe.Pointer(r.Uncompressed.Data), int(r.Uncompressed.Len))
	runtime.SetFinalizer(&uncompressed, func(_ *[]byte) {
		C.BufferFree(r.Uncompressed)
	})
	format = Format{
		Channels:   int(r.Format.Channels),
		BitDepth:   int(r.Format.BitDepth),
		SampleRate: int(r.Format.SampleRate),
	}
	return uncompressed, format, nil
}

// goBytes returns a slice backed by a C byte array.
//
// [1 << 30] means assume backing array is huge, and then slice into it
// with length.
func goBytes(ptr unsafe.Pointer, length int) []byte {
	return (*[1 << 30]byte)(ptr)[:length:length]
}

// cBytes returns a dynamic C byte array backed by a Go slice.
func cBytes(by []byte) (*C.uchar, C.uint) {
	return (*C.uchar)(unsafe.Pointer(&by[0])), C.uint(len(by))
}

// collectErrors unwraps all the errors in the chain and coalesces them
// into a single Go error.
func collectErrors(err *C.Error) error {
	var buf strings.Builder
	for first := err; err != nil; err = err.Err {
		if err.Str != nil {
			if err != first {
				buf.WriteString(": ")
			}
			buf.WriteString(strings.TrimSpace(C.GoString(err.Str)))
			if err.Code != 0 {
				buf.WriteString(fmt.Sprintf(" (%d)", err.Code))
			}
		}
	}
	str := strings.TrimSpace(buf.String())
	if str == "" {
		panic("error message is empty")
	}
	return errors.New(str)
}
