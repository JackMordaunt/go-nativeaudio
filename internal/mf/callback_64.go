//go:build windows && (amd64 || arm64)

package mf

import "syscall"

// onReadSampleTrampoline implements IMFSourceReaderCallback::OnReadSample:
//
//	HRESULT OnReadSample(HRESULT hrStatus, DWORD dwStreamIndex,
//	                     DWORD dwStreamFlags, LONGLONG llTimestamp,
//	                     IMFSample *pSample)
//
// Every argument occupies one pointer-sized slot here, so the 64-bit
// timestamp needs no special handling. It is ignored in any case, because
// the timestamp is read back from the sample itself. That keeps this
// signature the only thing that differs between word sizes.
var onReadSampleTrampoline = syscall.NewCallback(
	func(this *callback, status uintptr, streamIndex, flags uint32, timestamp int64, sample *IMFSample) uintptr {
		return guard("IMFSourceReaderCallback::OnReadSample", func() HRESULT {
			return this.onReadSample(HRESULT(status), flags, sample)
		})
	},
)
