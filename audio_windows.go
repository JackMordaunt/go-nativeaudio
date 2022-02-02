//go:build windows && cgo

package nativeaudio

// -g: add to CFLAGS to include dwarf debug data
// -O: optimization level 0, 1, 2, 3, s

/*
#cgo CFLAGS: -Werror -g -O3
#cgo LDFLAGS: -lwinmm -lmf -lmfplat  -lmfuuid -loleaut32 -limm32 -lversion -lwindowsapp -lmfreadwrite -lshlwapi
#include "audio_windows.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

func start() error {
	if err := C.StartMediaFramework(); err != nil {
		return fmt.Errorf("initializing Windows Media Framework: %w", collectErrors(err))
	}
	return nil
}

func end() error {
	if err := C.EndMediaFramework(); err != nil {
		return fmt.Errorf("shutting down Windows Media Framework: %w", collectErrors(err))
	}
	return nil
}

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
// uncompressed is a read-only slice backed by a C buffer. Do not mutate.
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
	defer C.BufferFree(r.Uncompressed)
	uncompressed = C.GoBytes(unsafe.Pointer(r.Uncompressed.Data), C.int(r.Uncompressed.Len))
	format = Format{
		SampleRate: int(r.Format.SampleRate),
		BitDepth:   int(r.Format.BitDepth),
		Channels:   int(r.Format.Channels),
	}
	return uncompressed, format, nil
}

// decode compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
//
// uncompressed is a read-only slice backed by a C buffer. Do not mutate.
func decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	r := C.Decode((*C.uchar)(C.CBytes(compressed)), C.uint(len(compressed)))
	if r.Err != nil && r.Err.Str != nil {
		defer C.ErrorFree(r.Err)
		return nil, format, collectErrors(r.Err)
	}
	defer C.BufferFree(r.Uncompressed)
	uncompressed = C.GoBytes(unsafe.Pointer(r.Uncompressed.Data), C.int(r.Uncompressed.Len))
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
