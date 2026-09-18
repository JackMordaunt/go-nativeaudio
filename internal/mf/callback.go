//go:build windows

package mf

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// readTimeout bounds the wait for one asynchronous read.
//
// A healthy decode delivers samples in milliseconds. Media Foundation can
// block forever on a malformed stream, and in synchronous mode that hang
// is unrecoverable: the blocked call owns the calling thread and there is
// nothing to interrupt it. Driving the reader asynchronously makes the
// wait ours to abandon.
const readTimeout = 10 * time.Second

// errReadTimeout reports that Media Foundation never answered a read.
var errReadTimeout = errors.New("timed out waiting for the decoder")

// HRESULTs the callback needs beyond S_OK.
const (
	E_NOINTERFACE HRESULT = 0x80004002
	E_POINTER     HRESULT = 0x80004003
	E_FAIL        HRESULT = 0x80004005
)

var (
	IID_IUnknown                = GUID{0x00000000, 0x0000, 0x0000, [8]uint8{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	IID_IMFSourceReaderCallback = GUID{0xdeec8d99, 0xfa1d, 0x4d82, [8]uint8{0x84, 0xc2, 0x2c, 0x89, 0x69, 0x94, 0x48, 0x67}}

	MF_SOURCE_READER_ASYNC_CALLBACK = GUID{0x1e3dbeac, 0xbb43, 0x4c35, [8]uint8{0xb5, 0x07, 0xcd, 0x64, 0x44, 0x64, 0xc9, 0x65}}
)

// failed reports whether an HRESULT denotes failure, which is the sign bit
// of the 32-bit code.
func failed(hr HRESULT) bool {
	return int32(uint32(hr)) < 0
}

// IUnknownVtbl is the head of every COM vtable.
type IUnknownVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

// sourceReaderCallbackVtbl is the vtable of IMFSourceReaderCallback.
type sourceReaderCallbackVtbl struct {
	IUnknownVtbl
	OnReadSample uintptr
	OnFlush      uintptr
	OnEvent      uintptr
}

// readResult is one delivery from OnReadSample. Its sample carries a
// reference taken by the callback, which the receiver must release.
type readResult struct {
	status HRESULT
	flags  uint32
	sample *IMFSample
}

// callback implements IMFSourceReaderCallback so the source reader can be
// driven asynchronously.
//
// A COM interface pointer points at a struct whose first word points to a
// vtable of function pointers. Handing Media Foundation the address of a
// Go struct whose first field is that vtable pointer therefore makes it a
// usable COM object, and each trampoline receives the struct back as its
// first argument. The vtable is a package-level singleton because
// [syscall.NewCallback] never frees what it creates.
//
// Media Foundation invokes the callback on its own worker thread, so
// deliveries arrive over a channel rather than being handled in place.
type callback struct {
	vtbl *sourceReaderCallbackVtbl // must remain the first field.

	refs atomic.Int32
	pin  runtime.Pinner

	// ch holds at most one undelivered read. Reads are issued one at a
	// time, so a deeper buffer would only hide a protocol mistake.
	ch chan readResult
}

// newCallback returns a callback with one reference, pinned so Media
// Foundation may hold its address.
func newCallback() *callback {
	c := &callback{vtbl: sourceReaderCallbackVtable, ch: make(chan readResult, 1)}
	c.refs.Store(1)
	c.pin.Pin(c)
	return c
}

// AddRef implements IUnknown::AddRef.
func (c *callback) AddRef() uint32 {
	return uint32(c.refs.Add(1))
}

// Release implements IUnknown::Release, unpinning on the last reference.
//
// Media Foundation holds its own reference for as long as it might still
// call us, so the object outlives the stream that created it whenever a
// read is still outstanding.
func (c *callback) Release() uint32 {
	n := c.refs.Add(-1)
	if n == 0 {
		c.drain()
		c.pin.Unpin()
	}
	return uint32(n)
}

// QueryInterface implements IUnknown::QueryInterface.
func (c *callback) QueryInterface(riid *GUID, ppv *unsafe.Pointer) HRESULT {
	if ppv == nil {
		return E_POINTER
	}
	if riid == nil || (*riid != IID_IUnknown && *riid != IID_IMFSourceReaderCallback) {
		*ppv = nil
		return E_NOINTERFACE
	}
	*ppv = unsafe.Pointer(c)
	c.AddRef()
	return S_OK
}

// onReadSample takes delivery of one read.
//
// The sample is only guaranteed for the duration of this call, so it is
// retained before being handed over. When nothing is waiting, which is
// what happens once the reader has given up on a read, the sample is
// released here rather than leaked.
func (c *callback) onReadSample(status HRESULT, flags uint32, sample *IMFSample) HRESULT {
	sample.AddRef()
	select {
	case c.ch <- readResult{status: status, flags: flags, sample: sample}:
	default:
		sample.Release()
	}
	return S_OK
}

// wait blocks for the next delivery, giving up after d.
func (c *callback) wait(d time.Duration) (readResult, error) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case r := <-c.ch:
		return r, nil
	case <-timer.C:
		return readResult{}, errReadTimeout
	}
}

// drain releases any sample left undelivered.
func (c *callback) drain() {
	for {
		select {
		case r := <-c.ch:
			r.sample.Release()
		default:
			return
		}
	}
}

// guard runs a COM method body and turns a panic into a failed HRESULT.
//
// A panic unwinding out of a [syscall.NewCallback] trampoline crosses C
// frames and takes the process down with it. COM callers expect failure
// as a status code, so report it as one.
func guard(method string, body func() HRESULT) (hr HRESULT) {
	defer func() {
		if p := recover(); p != nil {
			fmt.Fprintf(os.Stderr, "nativeaudio: panic in %s: %v\n%s\n", method, p, debug.Stack())
			hr = E_FAIL
		}
	}()
	return body()
}

// sourceReaderCallbackVtable is shared by every callback instance. The
// trampolines recover the instance from the pointer COM passes back.
var sourceReaderCallbackVtable = &sourceReaderCallbackVtbl{
	IUnknownVtbl: IUnknownVtbl{
		QueryInterface: syscall.NewCallback(func(this *callback, riid *GUID, ppv *unsafe.Pointer) uintptr {
			return guard("IMFSourceReaderCallback::QueryInterface", func() HRESULT { return this.QueryInterface(riid, ppv) })
		}),
		AddRef: syscall.NewCallback(func(this *callback) uintptr {
			return guard("IMFSourceReaderCallback::AddRef", func() HRESULT { return HRESULT(this.AddRef()) })
		}),
		Release: syscall.NewCallback(func(this *callback) uintptr {
			return guard("IMFSourceReaderCallback::Release", func() HRESULT { return HRESULT(this.Release()) })
		}),
	},
	OnReadSample: onReadSampleTrampoline,
	// Nothing is flushed and no events are acted on, but both slots must
	// be populated and must succeed.
	OnFlush: syscall.NewCallback(func(this *callback, streamIndex uint32) uintptr {
		return S_OK
	}),
	OnEvent: syscall.NewCallback(func(this *callback, streamIndex uint32, event uintptr) uintptr {
		return S_OK
	}),
}
