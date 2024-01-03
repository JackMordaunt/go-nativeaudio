//go:build windows && cgo

package nativeaudio

// -g: add to CFLAGS to include dwarf debug data
// -O: optimization level 0, 1, 2, 3, s

/*
#cgo CFLAGS: -Werror -g -O3
#cgo LDFLAGS: -lwinmm -lmf -lmfplat  -lmfuuid -loleaut32 -limm32 -lversion -lwindowsapp -lmfreadwrite -lshlwapi
#include "audio_windows.h"
#include <crtdbg.h>
#include <stdlib.h>
#include <stdio.h>
#include <windows.h>
#include <winbase.h>
#include <combaseapi.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mferror.h>
#include <initguid.h>
#include <wmcodecdsp.h>
#include <mmdeviceapi.h>
#include <mfreadwrite.h>
#include <shlwapi.h>
#include <assert.h>
#include <stdint.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func start() error {
	if hr := C.MFStartup(C.MF_VERSION, C.MFSTARTUP_LITE); hr != C.S_OK {
		return fmt.Errorf("initializing Media Framework: %w", MFErr{Code: hr})
	}
	return nil
}

func end() error {
	if hr := C.MFShutdown(); hr != C.S_OK {
		return fmt.Errorf("shutting down Media Framework: %w", MFErr{Code: hr})
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
func load(path string) (uncompressed []byte, format Format, err error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	r := C.Load(cPath)

	if r.Err != nil {
		defer C.ErrorFree(r.Err)
		return nil, Format{}, collectErrors(r.Err)
	}

	defer C.BufferFree(r.Uncompressed)

	uncompressed = GoSlice((*byte)(r.Uncompressed.Data), int64(r.Uncompressed.Len))

	format = Format{
		SampleRate: int(r.Format.SampleRate),
		BitDepth:   int(r.Format.BitDepth),
		Channels:   int(r.Format.Channels),
	}

	runtime.KeepAlive(path)

	return uncompressed, format, nil
}

// decode compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	data := compressed

	r := C.Decode((*C.uchar)(unsafe.SliceData(data)), C.uint(len(compressed)))

	if r.Err != nil && r.Err.Str != nil {
		defer C.ErrorFree(r.Err)
		return nil, format, collectErrors(r.Err)
	}

	defer C.BufferFree(r.Uncompressed)

	uncompressed = GoSlice((*byte)(r.Uncompressed.Data), int64(r.Uncompressed.Len))

	format = Format{
		Channels:   int(r.Format.Channels),
		BitDepth:   int(r.Format.BitDepth),
		SampleRate: int(r.Format.SampleRate),
	}

	runtime.KeepAlive(compressed)

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

// GoSlice takes a native array and returns a Go managed slice via a memory copy.
// The caller is responsible for freeing the native memory.
func GoSlice[T any](t *T, size int64) []T {
	src := unsafe.Slice(t, size)
	dst := make([]T, size)
	copy(dst, src)
	return dst
}

// MFErr is a Media Foundation error that can render a formatted message.
type MFErr struct {
	Code C.HRESULT
}

func (err MFErr) Error() string {
	outBuf := new(C.ushort)

	size := C.FormatMessageW(
		C.FORMAT_MESSAGE_ALLOCATE_BUFFER|C.FORMAT_MESSAGE_FROM_SYSTEM,
		nil,
		C.ulong(err.Code),
		0,
		outBuf,
		0,
		nil,
	)
	if outBuf == nil || size == 0 {
		return fmt.Sprintf("<cannot render string for HRESULT=%x>", err.Code)
	}

	defer C.LocalFree((C.HANDLE)(unsafe.Pointer(outBuf)))

	return windows.UTF16ToString(GoSlice((*uint16)(unsafe.Pointer(outBuf)), int64(size)))
}
