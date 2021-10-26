//go:build windows && cgo

package nativeaudio

// -g: includes dwarf debug data

/*
#cgo CFLAGS: -Wall -Werror -g
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
		defer C.free(unsafe.Pointer(err))
		return fmt.Errorf(C.GoString(err.Str))
	}
	return nil
}

// load raw pcm data from the Windows Media Foundation.
//
// PERF(jfm): we can optimize this by allocating the buffer from Go,
// and passing it in for C to fill up. It would require more
// orchestration, but would save the copy. At the moment, C allocates
// its own buffer, we then copy the data and free the C buffer.
func load(path string) ([]byte, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	result := C.Load(cPath)
	if result.Err != nil {
		// defer C.ErrorFree(result.Err)
		return nil, collectErrors(result.Err)
	}
	buffer := (*C.Buffer)(result.Value)
	defer C.BufferFree(buffer)
	return C.GoBytes(unsafe.Pointer(buffer.Data), buffer.Len), nil
}

// decode compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
//
// TODO(jfm) [perf]: avoid copying buffer in (CBytes does a copy).
func decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	defer runtime.KeepAlive(compressed)
	r := C.Decode((*C.uchar)(C.CBytes(compressed)), C.uint(len(compressed)))
	if r.Err != nil && r.Err.Str != nil {
		// TODO(jfm): Free result.
		return nil, format, collectErrors(r.Err)
	}
	defer C.BufferFree(r.Uncompressed)
	uncompressed = C.GoBytes(unsafe.Pointer(r.Uncompressed.Data), r.Uncompressed.Len)
	format = Format{
		Channels:   int(r.Format.Channels),
		BitDepth:   int(r.Format.BitDepth),
		SampleRate: int(r.Format.SampleRate),
	}
	return uncompressed, format, nil
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
