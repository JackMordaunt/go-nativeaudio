//go:build windows && !amd64 && !arm64

package mf

import "syscall"

// onReadSampleTrampoline implements IMFSourceReaderCallback::OnReadSample.
//
// See the 64-bit variant for the interface signature. Where a word is four
// bytes wide the LONGLONG timestamp is passed as two slots, so it arrives
// as two halves and is ignored, exactly as it is there.
var onReadSampleTrampoline = syscall.NewCallback(
	func(this *callback, status uintptr, streamIndex, flags, timestampLow, timestampHigh uint32, sample *IMFSample) uintptr {
		return guard("IMFSourceReaderCallback::OnReadSample", func() HRESULT {
			return this.onReadSample(HRESULT(status), flags, sample)
		})
	},
)
