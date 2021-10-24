//go:build windows && cgo

package nativeaudio

/*
#cgo CFLAGS: -Wall -Werror
#cgo LDFLAGS: -lWinmm -lMFPlat -lMf -lMfuuid -loleaut32 -limm32 -lversion -lWindowsApp -lMfreadwrite
#include "audio_windows.h"
*/
import "C"
import (
	"fmt"
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
		defer C.ErrorFree(result.Err)
		return nil, fmt.Errorf(C.GoString(result.Err.Str))
	}
	buffer := (*C.Buffer)(result.Value)
	defer C.BufferFree(buffer)
	return C.GoBytes(unsafe.Pointer(buffer.Data), buffer.Len), nil
}
