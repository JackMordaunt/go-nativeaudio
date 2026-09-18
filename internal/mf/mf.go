//go:build windows

// Package mf wraps the minimal subset of Windows Media Foundation needed
// to decode compressed audio into PCM.
//
// Only Startup, Shutdown, Decode and Format are meant for callers. The
// remaining identifiers mirror the SDK headers so they can be checked
// against them, and are exported only for that readability; this package
// is internal and they are not part of the public API.
package mf

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Format describes the PCM produced by Decode.
type Format struct {
	SampleRate     int // samples per second.
	Channels       int // number channels.
	BytesPerSample int // bytes per sample.
}

// Startup initialises Media Foundation and resolves the entry points
// this package uses. Call Shutdown once per successful Startup.
func Startup() error {
	return MFStartup(MF_VERSION, MFSTARTUP_LITE)
}

// Shutdown releases Media Foundation.
func Shutdown() error {
	return MFShutdown()
}

// Decode compressed audio, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func Decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	stream := SHCreateMemStream(unsafe.SliceData(compressed), len(compressed))
	if stream == nil {
		return nil, format, fmt.Errorf("could not allocate IStream")
	}
	defer stream.Release()

	// We need to adapt the generic IStream to a Media Foundation stream type.
	var mfByteStream *IMFByteStream
	if err := MFCreateMFByteStreamOnStream(stream, &mfByteStream); err != nil {
		return nil, format, fmt.Errorf("creating MFByteStream from IStream: %w", err)
	}
	defer mfByteStream.Release()

	// Attributes to configure the source reader with; specifically, enable hardware codecs.
	var attributes *IMFAttributes
	if err := MFCreateAttributes(&attributes, 1); err != nil {
		return nil, format, fmt.Errorf("creating attributes to apply to source reader: %w", err)
	}
	defer attributes.Release()

	if err := attributes.SetUINT32(&MF_READWRITE_ENABLE_HARDWARE_TRANSFORMS, 1); err != nil {
		return nil, format, fmt.Errorf("enabling hardware transforms: %w", err)
	}

	// Create the source reader using the byte stream.
	var mfSourceReader *IMFSourceReader
	if err := MFCreateSourceReaderFromByteStream(mfByteStream, attributes, &mfSourceReader); err != nil {
		return nil, format, fmt.Errorf("creating IMFSourceReader from IMFByteStream: %w", err)
	}
	defer mfSourceReader.Release()

	if err := configureAudioStream(mfSourceReader); err != nil {
		return nil, format, fmt.Errorf("configuring audio stream: %w", err)
	}

	format, err = getSourceReaderFormat(mfSourceReader)
	if err != nil {
		return nil, format, fmt.Errorf("getting format: %w", err)
	}

	buf, err := io.ReadAll(NewSourceReader(mfSourceReader))
	if err != nil {
		return nil, format, err
	}

	runtime.KeepAlive(compressed)

	return buf, format, nil
}

// configureAudioStream selects the first audio stream and configures it output PCM.
func configureAudioStream(pReader *IMFSourceReader) error {
	var pPartialType *IMFMediaType
	// Create a partial media type that specifies uncompressed PCM audio.
	if err := MFCreateMediaType(&pPartialType); err != nil {
		return fmt.Errorf("creating media type: %w", err)
	}
	defer pPartialType.Release()

	if err := pPartialType.SetGUID(&MF_MT_MAJOR_TYPE, &MFMediaType_Audio); err != nil {
		return fmt.Errorf("setting major type: %w", err)
	}

	if err := pPartialType.SetGUID(&MF_MT_SUBTYPE, &MFAudioFormat_PCM); err != nil {
		return fmt.Errorf("setting sub type: %w", err)
	}

	// Select the first audio stream, and deselect all other streams.
	if err := pReader.SetStreamSelection(MF_SOURCE_READER_ALL_STREAMS, false); err != nil {
		return fmt.Errorf("deselecting audio streams: %w", err)
	}
	if err := pReader.SetStreamSelection(MF_SOURCE_READER_FIRST_AUDIO_STREAM, true); err != nil {
		return fmt.Errorf("selecting first audio stream: %w", err)
	}

	// Set this type on the source reader. The source reader will load the necessary decoder.
	if err := pReader.SetCurrentMediaType(MF_SOURCE_READER_FIRST_AUDIO_STREAM, pPartialType); err != nil {
		return fmt.Errorf("setting media type on source reader: %w", err)
	}

	return nil
}

// getSourceReaderFormat returns the audio format configured for the source reader.
func getSourceReaderFormat(sr *IMFSourceReader) (f Format, _ error) {
	var mfMediaType *IMFMediaType
	// Get the complete uncompressed format.
	if err := sr.GetCurrentMediaType(MF_SOURCE_READER_FIRST_AUDIO_STREAM, &mfMediaType); err != nil {
		return f, fmt.Errorf("getting the current media type: %w", err)
	}
	defer mfMediaType.Release()

	return getFormat(mfMediaType)
}

// getFormat extracts the audio format from a media type.
func getFormat(mt *IMFMediaType) (f Format, _ error) {
	var (
		numChannels   uint32
		sampleRate    uint32
		bitsPerSample uint32
	)

	if err := mt.GetUINT32(&MF_MT_AUDIO_NUM_CHANNELS, &numChannels); err != nil {
		return f, fmt.Errorf("getting numChannels for the media type: %w", err)
	}
	if err := mt.GetUINT32(&MF_MT_AUDIO_SAMPLES_PER_SECOND, &sampleRate); err != nil {
		return f, fmt.Errorf("getting sampleRate for the media type: %w", err)
	}
	if err := mt.GetUINT32(&MF_MT_AUDIO_BITS_PER_SAMPLE, &bitsPerSample); err != nil {
		return f, fmt.Errorf("getting bitsPerSample for the media type: %w", err)
	}

	f = Format{
		SampleRate:     int(sampleRate),
		Channels:       int(numChannels),
		BytesPerSample: int(bitsPerSample / 8),
	}

	return f, nil
}

// SourceReader wraps an IMFSourceReader and implements [io.Reader].
type SourceReader struct {
	source *IMFSourceReader

	sample  *IMFSample      // sample object containing one or more streams
	buffer  *IMFMediaBuffer // buffer object containing the raw buffer
	chunk   *byte           // start of chunk of audio data
	chunkSz uint32          // size of chunk

	prevTimestamp int64 // timestamp of the last emitted sample.
	hasPrev       bool  // whether any sample has been emitted yet.

	data []byte // Go view of the audio data, backed by native memory.
}

// NewSourceReader allocates a [SourceReader].
// The caller is responsible for releasing the underlying [IMFSourceReader].
func NewSourceReader(r *IMFSourceReader) *SourceReader {
	return &SourceReader{source: r}
}

func (s *SourceReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	// Write residual data into p before requesting more.
	if len(s.data) > 0 {
		return s.copyInto(p), nil
	}

	ok, err := s.next()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, io.EOF
	}

	return s.copyInto(p), nil
}

// next reads the next audio sample into s, returning false at end of
// stream. Any error from the source reader ends the stream.
func (s *SourceReader) next() (bool, error) {
	for {
		var (
			flags     uint32
			timestamp int64
			sample    *IMFSample
		)

		err := s.source.ReadSample(MF_SOURCE_READER_FIRST_AUDIO_STREAM, 0, nil, &flags, &timestamp, &sample)

		// A sample can accompany a flag we treat as terminal, so release
		// it before deciding whether to stop.
		if err != nil || flags&(MF_SOURCE_READERF_ERROR|MF_SOURCE_READERF_ENDOFSTREAM|MF_SOURCE_READERF_CURRENTMEDIATYPECHANGED) != 0 {
			sample.Release()
		}
		if err != nil {
			return false, fmt.Errorf("reading sample: %w", err)
		}
		if flags&MF_SOURCE_READERF_ERROR != 0 {
			return false, fmt.Errorf("reading sample: source reader reported an error")
		}
		if flags&MF_SOURCE_READERF_CURRENTMEDIATYPECHANGED != 0 {
			return false, fmt.Errorf("reading sample: media type changed mid-stream")
		}
		if flags&MF_SOURCE_READERF_ENDOFSTREAM != 0 {
			return false, nil
		}

		// The reader can legitimately return no sample and no terminal
		// flag (for example, while a decoder is buffering). Ask again.
		if sample == nil {
			continue
		}

		// Skip samples that repeat the previous timestamp. ReadSample can
		// produce more than one sample at time 0; emitting all of them
		// produces larger output and audible artifacts.
		if s.hasPrev && timestamp == s.prevTimestamp {
			sample.Release()
			continue
		}

		s.sample = sample
		s.prevTimestamp = timestamp
		s.hasPrev = true
		break
	}

	if err := s.sample.ConvertToContiguousBuffer(&s.buffer); err != nil {
		s.sample.Release()
		s.sample = nil
		return false, fmt.Errorf("converting sample to contiguous buffer: %w", err)
	}
	if err := s.buffer.Lock(&s.chunk, nil, &s.chunkSz); err != nil {
		s.buffer.Release()
		s.sample.Release()
		s.buffer, s.sample = nil, nil
		return false, fmt.Errorf("locking sample buffer: %w", err)
	}

	// Make a Go slice view of the data for easy consumption.
	s.data = unsafe.Slice(s.chunk, s.chunkSz)

	return true, nil
}

// copyInto copies audio data into p, unlocking the memory once fully copied.
func (s *SourceReader) copyInto(p []byte) int {
	n := copy(p, s.data)

	s.data = s.data[n:]

	if len(s.data) == 0 {
		s.buffer.Unlock()
		s.buffer.Release()
		s.sample.Release()
		s.buffer, s.sample = nil, nil
		s.data = nil
	}

	return n
}

/*
	The following contains the minimal set of definitions we need to decode
	audio using Media Framework.

	Definitions are derived from appropriate SDK headers.
*/

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]uint8
}

type HRESULT = uintptr

const (
	S_OK                                      = 0x0
	MFSTARTUP_LITE                            = 0x1
	MF_VERSION                                = 0x20070
	MF_SOURCE_READERF_CURRENTMEDIATYPECHANGED = 0x20
	MF_SOURCE_READERF_ENDOFSTREAM             = 0x2
	MF_SOURCE_READERF_ERROR                   = 0x1
	MF_SOURCE_READER_ALL_STREAMS              = 0xfffffffe
	MF_SOURCE_READER_FIRST_AUDIO_STREAM       = 0xfffffffd
)

var (
	MF_READWRITE_ENABLE_HARDWARE_TRANSFORMS = GUID{0xa634a91c, 0x822b, 0x41b9, [8]uint8{0xa4, 0x94, 0x4d, 0xe4, 0x64, 0x36, 0x12, 0xb0}}

	MF_MT_AUDIO_NUM_CHANNELS             = GUID{0x37e48bf5, 0x645e, 0x4c5b, [8]uint8{0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a}}
	MF_MT_AUDIO_SAMPLES_PER_SECOND       = GUID{0x5faeeae7, 0x0290, 0x4c31, [8]uint8{0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba}}
	MF_MT_AUDIO_FLOAT_SAMPLES_PER_SECOND = GUID{0xfb3b724a, 0xcfb5, 0x4319, [8]uint8{0xae, 0xfe, 0x6e, 0x42, 0xb2, 0x40, 0x61, 0x32}}
	MF_MT_AUDIO_AVG_BYTES_PER_SECOND     = GUID{0x1aab75c8, 0xcfef, 0x451c, [8]uint8{0xab, 0x95, 0xac, 0x03, 0x4b, 0x8e, 0x17, 0x31}}
	MF_MT_AUDIO_BLOCK_ALIGNMENT          = GUID{0x322de230, 0x9eeb, 0x43bd, [8]uint8{0xab, 0x7a, 0xff, 0x41, 0x22, 0x51, 0x54, 0x1d}}
	MF_MT_AUDIO_BITS_PER_SAMPLE          = GUID{0xf2deb57f, 0x40fa, 0x4764, [8]uint8{0xaa, 0x33, 0xed, 0x4f, 0x2d, 0x1f, 0xf6, 0x69}}
	MF_MT_AUDIO_VALID_BITS_PER_SAMPLE    = GUID{0xd9bf8d6a, 0x9530, 0x4b7c, [8]uint8{0x9d, 0xdf, 0xff, 0x6f, 0xd5, 0x8b, 0xbd, 0x06}}
	MF_MT_AUDIO_SAMPLES_PER_BLOCK        = GUID{0xaab15aac, 0xe13a, 0x4995, [8]uint8{0x92, 0x22, 0x50, 0x1e, 0xa1, 0x5c, 0x68, 0x77}}
	MF_MT_AUDIO_CHANNEL_MASK             = GUID{0x55fb5765, 0x644a, 0x4caf, [8]uint8{0x84, 0x79, 0x93, 0x89, 0x83, 0xbb, 0x15, 0x88}}

	MF_MT_MAJOR_TYPE = GUID{0x48eba18e, 0xf8c9, 0x4687, [8]uint8{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	MF_MT_SUBTYPE    = GUID{0xf7e34c9a, 0x42e8, 0x4714, [8]uint8{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}

	MFMediaType_Audio = GUID{0x73647561, 0x0000, 0x0010, [8]uint8{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	MFAudioFormat_PCM = GUID{0x00000001, 0x0000, 0x0010, [8]uint8{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
)

type IMFByteStream struct {
	VTable *IMFByteStreamVTable
}

type IMFByteStreamVTable struct {
	QueryInterface     uintptr
	AddRef             uintptr
	Release            uintptr
	GetCapabilities    uintptr
	GetLength          uintptr
	SetLength          uintptr
	GetCurrentPosition uintptr
	SetCurrentPosition uintptr
	IsEndOfStream      uintptr
	Read               uintptr
	BeginRead          uintptr
	EndRead            uintptr
	Write              uintptr
	BeginWrite         uintptr
	EndWrite           uintptr
	Seek               uintptr
	Flush              uintptr
	Close              uintptr
}

func (v *IMFByteStream) Release() error {
	if v == nil {
		return nil
	}
	r, _, _ := syscall.SyscallN(
		v.VTable.Release,
		uintptr(unsafe.Pointer(v)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

type IStream struct {
	VTable *IStreamVTable
}

type IStreamVTable struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	Read           uintptr
	Write          uintptr
	Seek           uintptr
	SetSize        uintptr
	CopyTo         uintptr
	Commit         uintptr
	Revert         uintptr
	LockRegion     uintptr
	UnlockRegion   uintptr
	Stat           uintptr
	Clone          uintptr
}

func (v *IStream) Release() error {
	if v == nil {
		return nil
	}
	r, _, _ := syscall.SyscallN(
		v.VTable.Release,
		uintptr(unsafe.Pointer(v)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

type IMFAttributes struct {
	VTable *IMFAttributesVTable
}

type IMFAttributesVTable struct {
	QueryInterface     uintptr
	AddRef             uintptr
	Release            uintptr
	GetItem            uintptr
	GetItemType        uintptr
	CompareItem        uintptr
	Compare            uintptr
	GetUINT32          uintptr
	GetUINT64          uintptr
	GetDouble          uintptr
	GetGUID            uintptr
	GetStringLength    uintptr
	GetString          uintptr
	GetAllocatedString uintptr
	GetBlobSize        uintptr
	GetBlob            uintptr
	GetAllocatedBlob   uintptr
	GetUnknown         uintptr
	SetItem            uintptr
	DeleteItem         uintptr
	DeleteAllItems     uintptr
	SetUINT32          uintptr
	SetUINT64          uintptr
	SetDouble          uintptr
	SetGUID            uintptr
	SetString          uintptr
	SetBlob            uintptr
	SetUnknown         uintptr
	LockStore          uintptr
	UnlockStore        uintptr
	GetCount           uintptr
	GetItemByIndex     uintptr
	CopyAllItems       uintptr
}

func (v *IMFAttributes) Release() error {
	if v == nil {
		return nil
	}
	r, _, _ := syscall.SyscallN(
		v.VTable.Release,
		uintptr(unsafe.Pointer(v)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFAttributes) SetUINT32(guid *GUID, unValue uint32) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.SetUINT32,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(guid)),
		uintptr(unValue),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

type IMFMediaType struct {
	VTable *IMFMediaTypeVTable
}

type IMFMediaTypeVTable struct {
	QueryInterface     uintptr
	AddRef             uintptr
	Release            uintptr
	GetItem            uintptr
	GetItemType        uintptr
	CompareItem        uintptr
	Compare            uintptr
	GetUINT32          uintptr
	GetUINT64          uintptr
	GetDouble          uintptr
	GetGUID            uintptr
	GetStringLength    uintptr
	GetString          uintptr
	GetAllocatedString uintptr
	GetBlobSize        uintptr
	GetBlob            uintptr
	GetAllocatedBlob   uintptr
	GetUnknown         uintptr
	SetItem            uintptr
	DeleteItem         uintptr
	DeleteAllItems     uintptr
	SetUINT32          uintptr
	SetUINT64          uintptr
	SetDouble          uintptr
	SetGUID            uintptr
	SetString          uintptr
	SetBlob            uintptr
	SetUnknown         uintptr
	LockStore          uintptr
	UnlockStore        uintptr
	GetCount           uintptr
	GetItemByIndex     uintptr
	CopyAllItems       uintptr
	GetMajorType       uintptr
	IsCompressedFormat uintptr
	IsEqual            uintptr
	GetRepresentation  uintptr
	FreeRepresentation uintptr
}

func (v *IMFMediaType) Release() error {
	if v == nil {
		return nil
	}
	r, _, _ := syscall.SyscallN(
		v.VTable.Release,
		uintptr(unsafe.Pointer(v)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFMediaType) SetGUID(guid *GUID, value *GUID) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.SetGUID,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(guid)),
		uintptr(unsafe.Pointer(value)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFMediaType) GetUINT32(guid *GUID, punValue *uint32) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.GetUINT32,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(guid)),
		uintptr(unsafe.Pointer(punValue)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

type IMFSourceReader struct {
	VTable *IMFSourceReaderVtbl
}

type IMFSourceReaderVtbl struct {
	QueryInterface           uintptr
	AddRef                   uintptr
	Release                  uintptr
	GetStreamSelection       uintptr
	SetStreamSelection       uintptr
	GetNativeMediaType       uintptr
	GetCurrentMediaType      uintptr
	SetCurrentMediaType      uintptr
	SetCurrentPosition       uintptr
	ReadSample               uintptr
	Flush                    uintptr
	GetServiceForStream      uintptr
	GetPresentationAttribute uintptr
}

func (v *IMFSourceReader) Release() error {
	if v == nil {
		return nil
	}
	r, _, _ := syscall.SyscallN(
		v.VTable.Release,
		uintptr(unsafe.Pointer(v)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFSourceReader) SetStreamSelection(index uint32, selected bool) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.SetStreamSelection,
		uintptr(unsafe.Pointer(v)),
		uintptr(index),
		uintptr(boolToInt(selected)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFSourceReader) SetCurrentMediaType(index uint32, mt *IMFMediaType) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.SetCurrentMediaType,
		uintptr(unsafe.Pointer(v)),
		uintptr(index),
		uintptr(0),
		uintptr(unsafe.Pointer(mt)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFSourceReader) GetCurrentMediaType(index uint32, mt **IMFMediaType) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.GetCurrentMediaType,
		uintptr(unsafe.Pointer(v)),
		uintptr(index),
		uintptr(unsafe.Pointer(mt)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFSourceReader) ReadSample(index, controlFlags uint32, actualIndex *uint32, streamFlags *uint32, timestamp *int64, sample **IMFSample) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.ReadSample,
		uintptr(unsafe.Pointer(v)),
		uintptr(index),
		uintptr(controlFlags),
		uintptr(unsafe.Pointer(actualIndex)),
		uintptr(unsafe.Pointer(streamFlags)),
		uintptr(unsafe.Pointer(timestamp)),
		uintptr(unsafe.Pointer(sample)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

type IMFSample struct {
	VTable *IMFSampleVTable
}

type IMFSampleVTable struct {
	QueryInterface            uintptr
	AddRef                    uintptr
	Release                   uintptr
	GetItem                   uintptr
	GetItemType               uintptr
	CompareItem               uintptr
	Compare                   uintptr
	GetUINT32                 uintptr
	GetUINT64                 uintptr
	GetDouble                 uintptr
	GetGUID                   uintptr
	GetStringLength           uintptr
	GetString                 uintptr
	GetAllocatedString        uintptr
	GetBlobSize               uintptr
	GetBlob                   uintptr
	GetAllocatedBlob          uintptr
	GetUnknown                uintptr
	SetItem                   uintptr
	DeleteItem                uintptr
	DeleteAllItems            uintptr
	SetUINT32                 uintptr
	SetUINT64                 uintptr
	SetDouble                 uintptr
	SetGUID                   uintptr
	SetString                 uintptr
	SetBlob                   uintptr
	SetUnknown                uintptr
	LockStore                 uintptr
	UnlockStore               uintptr
	GetCount                  uintptr
	GetItemByIndex            uintptr
	CopyAllItems              uintptr
	GetSampleFlags            uintptr
	SetSampleFlags            uintptr
	GetSampleTime             uintptr
	SetSampleTime             uintptr
	GetSampleDuration         uintptr
	SetSampleDuration         uintptr
	GetBufferCount            uintptr
	GetBufferByIndex          uintptr
	ConvertToContiguousBuffer uintptr
	AddBuffer                 uintptr
	RemoveBufferByIndex       uintptr
	RemoveAllBuffers          uintptr
	GetTotalLength            uintptr
	CopyToBuffer              uintptr
}

func (v *IMFSample) Release() error {
	if v == nil {
		return nil
	}
	r, _, _ := syscall.SyscallN(
		v.VTable.Release,
		uintptr(unsafe.Pointer(v)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFSample) ConvertToContiguousBuffer(b **IMFMediaBuffer) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.ConvertToContiguousBuffer,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(b)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

type IMFMediaBuffer struct {
	VTable *IMFMediaBufferVTable
}

type IMFMediaBufferVTable struct {
	QueryInterface   uintptr
	AddRef           uintptr
	Release          uintptr
	Lock             uintptr
	Unlock           uintptr
	GetCurrentLength uintptr
	SetCurrentLength uintptr
	GetMaxLength     uintptr
}

func (v *IMFMediaBuffer) Release() error {
	if v == nil {
		return nil
	}
	r, _, _ := syscall.SyscallN(
		v.VTable.Release,
		uintptr(unsafe.Pointer(v)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFMediaBuffer) Lock(buf **byte, maxLength, currentLength *uint32) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.Lock,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(buf)),
		uintptr(unsafe.Pointer(maxLength)),
		uintptr(unsafe.Pointer(currentLength)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func (v *IMFMediaBuffer) Unlock() error {
	r, _, _ := syscall.SyscallN(
		v.VTable.Unlock,
		uintptr(unsafe.Pointer(v)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

/*
	The following defines exactly the functions required.
*/

var (
	_mfplat      *windows.DLL
	_shlwapi     *windows.DLL
	_mfreadwrite *windows.DLL

	_SHCreateMemStream *windows.Proc

	_MFStartup                    *windows.Proc
	_MFShutdown                   *windows.Proc
	_MFCreateMediaType            *windows.Proc
	_MFCreateAttributes           *windows.Proc
	_MFCreateMFByteStreamOnStream *windows.Proc

	_MFCreateSourceReaderFromByteStream *windows.Proc
)

var loadOnce sync.Once

// loadProcs resolves the DLLs and entry points once per process. The
// handles stay valid for the life of the process, so repeated Startup
// and Shutdown cycles reuse them.
func loadProcs() (err error) {
	_mfplat, err = windows.LoadDLL("Mfplat.dll")
	if err != nil {
		return fmt.Errorf("Mfplat.dll: %w", err)
	}
	_shlwapi, err = windows.LoadDLL("Shlwapi.dll")
	if err != nil {
		return fmt.Errorf("Shlwapi.dll: %w", err)
	}
	_mfreadwrite, err = windows.LoadDLL("Mfreadwrite.dll")
	if err != nil {
		return fmt.Errorf("Mfreadwrite.dll: %w", err)
	}
	_SHCreateMemStream, err = _shlwapi.FindProc("SHCreateMemStream")
	if err != nil {
		return fmt.Errorf("SHCreateMemStream: %w", err)
	}
	_MFStartup, err = _mfplat.FindProc("MFStartup")
	if err != nil {
		return fmt.Errorf("MFStartup: %w", err)
	}
	_MFShutdown, err = _mfplat.FindProc("MFShutdown")
	if err != nil {
		return fmt.Errorf("MFShutdown: %w", err)
	}
	_MFCreateMediaType, err = _mfplat.FindProc("MFCreateMediaType")
	if err != nil {
		return fmt.Errorf("MFCreateMediaType: %w", err)
	}
	_MFCreateAttributes, err = _mfplat.FindProc("MFCreateAttributes")
	if err != nil {
		return fmt.Errorf("MFCreateAttributes: %w", err)
	}
	_MFCreateMFByteStreamOnStream, err = _mfplat.FindProc("MFCreateMFByteStreamOnStream")
	if err != nil {
		return fmt.Errorf("MFCreateMFByteStreamOnStream: %w", err)
	}
	_MFCreateSourceReaderFromByteStream, err = _mfreadwrite.FindProc("MFCreateSourceReaderFromByteStream")
	if err != nil {
		return fmt.Errorf("MFCreateSourceReaderFromByteStream: %w", err)
	}

	return nil
}

// MFStartup initialises Media Foundation. The platform refcounts this
// against MFShutdown, so callers must pair them.
func MFStartup(version, flags uintptr) error {
	var loadErr error
	loadOnce.Do(func() { loadErr = loadProcs() })
	if loadErr != nil {
		return loadErr
	}

	r, _, _ := _MFStartup.Call(version, flags)
	if r != S_OK {
		return MFErr{Code: r}
	}

	return nil
}

func MFShutdown() error {
	r, _, _ := _MFShutdown.Call()
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func SHCreateMemStream(pInit *byte, cbInit int) *IStream {
	r, _, _ := _SHCreateMemStream.Call(
		uintptr(unsafe.Pointer(pInit)),
		uintptr(uint32(cbInit)),
	)
	if r == 0 {
		return nil
	}
	// r is a COM interface pointer owned by the shell, not Go memory, so
	// the round trip through uintptr is safe. Converting via the address
	// of r keeps go vet from flagging a possible misuse of unsafe.Pointer.
	return *(**IStream)(unsafe.Pointer(&r))
}

func MFCreateAttributes(out **IMFAttributes, size uint64) error {
	r, _, _ := _MFCreateAttributes.Call(
		uintptr(unsafe.Pointer(out)),
		uintptr(size),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func MFCreateMFByteStreamOnStream(s *IStream, out **IMFByteStream) error {
	r, _, _ := _MFCreateMFByteStreamOnStream.Call(
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(out)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func MFCreateSourceReaderFromByteStream(bs *IMFByteStream, attributes *IMFAttributes, out **IMFSourceReader) error {
	r, _, _ := _MFCreateSourceReaderFromByteStream.Call(
		uintptr(unsafe.Pointer(bs)),
		uintptr(unsafe.Pointer(attributes)),
		uintptr(unsafe.Pointer(out)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func MFCreateMediaType(out **IMFMediaType) error {
	r, _, _ := _MFCreateMediaType.Call(
		uintptr(unsafe.Pointer(out)),
	)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func boolToInt(b bool) int {
	const False = 0
	const True = 1
	if b {
		return True
	}
	return False
}

// MFErr is a Media Foundation error that can render a formatted message.
type MFErr struct {
	Code HRESULT
}

func (e MFErr) Error() string {
	out := make([]uint16, 300)
	size, err := windows.FormatMessage(
		windows.FORMAT_MESSAGE_FROM_SYSTEM|windows.FORMAT_MESSAGE_FROM_HMODULE|windows.FORMAT_MESSAGE_ARGUMENT_ARRAY,
		uintptr(_mfplat.Handle),
		uint32(e.Code),
		0,
		out,
		nil,
	)
	if err != nil {
		return fmt.Sprintf("code %x (<format message: %v>)", e.Code, err.Error())
	}
	// trim terminating \r and \n
	for ; size > 0 && (out[size-1] == '\n' || out[size-1] == '\r'); size-- {
	}
	return fmt.Sprintf("%s (code %x)", string(utf16.Decode(out[:size])), e.Code)
}
