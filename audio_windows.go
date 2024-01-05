package nativeaudio

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"git.sr.ht/~jackmordaunt/nativeaudio/internal"
	"github.com/ebitengine/oto/v3"
	"golang.org/x/sys/windows"
)

func start() error {
	return MFStartup(MF_VERSION, MFSTARTUP_LITE)
}

func end() error {
	return MFShutdown()
}

// play the audio file using Windows Media Foundation.
func play(path string) error {
	data, format, err := load(path)
	if err != nil {
		return fmt.Errorf("decoding: %w", err)
	}
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   format.SampleRate,
		ChannelCount: format.Channels,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return fmt.Errorf("starting playback context: %w", err)
	}
	<-ready
	done := make(chan any)
	player := ctx.NewPlayer(internal.NewTriggerReader(bytes.NewReader(data), func() { close(done) }))
	player.Play()
	<-done
	ctx.Suspend()
	return player.Close()
}

// load raw pcm data from the Windows Media Foundation.
func load(path string) (uncompressed []byte, format Format, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, format, fmt.Errorf("opening input file: %w", err)
	}
	defer f.Close()
	by, err := io.ReadAll(f)
	if err != nil {
		return nil, format, fmt.Errorf("buffering input file: %w", err)
	}
	return decode(by)
}

// decode compressed data, returning the uncompressed data as PCM data
// (s16le) and details about the PCM required to playback correctly.
func decode(compressed []byte) (uncompressed []byte, format Format, err error) {
	stream := SHCreateMemStream(unsafe.SliceData(compressed), len(compressed))
	if stream == nil {
		return nil, format, fmt.Errorf("could not allocate IStream")
	}
	defer stream.Release()

	// We need to adapt the generic IStream to a Media Foundation stream type.
	var mfByteStream *IMFByteStream
	defer mfByteStream.Release()

	if err := MFCreateMFByteStreamOnStream(stream, &mfByteStream); err != nil {
		return nil, format, fmt.Errorf("creating MFByteStream from IStream: %w", err)
	}

	// Attributes to configure the source reader with; specifically, enable hardware codecs.
	var attributes *IMFAttributes
	defer attributes.Release()

	if err := MFCreateAttributes(&attributes, 1); err != nil {
		return nil, format, fmt.Errorf("creating attributes to apply to source reader: %w", err)
	}

	if err := attributes.SetUINT32(&MF_READWRITE_ENABLE_HARDWARE_TRANSFORMS, 1); err != nil {
		return nil, format, fmt.Errorf("enabling hardware transforms: %w", err)
	}

	// Create the source reader using the byte stream.
	var mfSourceReader *IMFSourceReader
	defer mfSourceReader.Release()

	if err := MFCreateSourceReaderFromByteStream(mfByteStream, attributes, &mfSourceReader); err != nil {
		return nil, format, fmt.Errorf("creating IMFSourceReader from IMFByteStream: %w", err)
	}

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
	defer pPartialType.Release()

	// Create a partial media pUncompressedAutioTypee that specifies uncompressed PCM audio.
	if err := MFCreateMediaType(&pPartialType); err != nil {
		return fmt.Errorf("creating media type: %w", err)
	}

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
	defer mfMediaType.Release()

	// Get the complete uncompressed format.
	if err := sr.GetCurrentMediaType(MF_SOURCE_READER_FIRST_AUDIO_STREAM, &mfMediaType); err != nil {
		return f, fmt.Errorf("getting the current media type: %w", err)
	}

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
		SampleRate: int(sampleRate),
		Channels:   int(numChannels),
		BitDepth:   int(bitsPerSample / 8),
	}

	return f, nil
}

// SourceReader wraps an IMFSourceReader and implements [io.Reader].
type SourceReader struct {
	source *IMFSourceReader

	sample  *IMFSample      // sample object containing one or more streams
	buffer  *IMFMediaBuffer // buffer object containing the raw buffer
	chunk   *byte           // start of chunk of audio data
	chunkSz int64           // size of chunk

	prevTimestamp    int64
	currentTimestamp int64

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

	if !s.next() {
		return 0, io.EOF
	}

	return s.copyInto(p), nil
}

// next reads the next audio sample, returning true if found, or false if EOF.
func (s *SourceReader) next() bool {
	for {
		var flags int64

		// Read the next sample; skipping samples with matching time stamps.
		// For some reason ReadSample can produce more than one sample at time 0.
		// Emitting all of them produces largers files and audio artifacts.
		s.source.ReadSample(MF_SOURCE_READER_FIRST_AUDIO_STREAM, 0, nil, &flags, &s.currentTimestamp, &s.sample)

		if flags&MF_SOURCE_READERF_CURRENTMEDIATYPECHANGED != 0 {
			return false
		}

		if flags&MF_SOURCE_READERF_ENDOFSTREAM != 0 {
			return false
		}

		if s.sample == nil {
			continue
		}

		if s.currentTimestamp != s.prevTimestamp-1 {
			break
		}
	}

	s.prevTimestamp = s.currentTimestamp + 1

	s.sample.ConvertToContiguousBuffer(&s.buffer)
	s.buffer.Lock(&s.chunk, nil, &s.chunkSz)

	// Make a Go slice view of the data for easy consumption.
	s.data = unsafe.Slice((*byte)(s.chunk), s.chunkSz)

	return true
}

// copyInto copies audio data into p, unlocking the memory once fully copied.
func (s *SourceReader) copyInto(p []byte) int {
	n := copy(p, s.data)

	s.data = s.data[n:]

	if len(s.data) == 0 {
		s.buffer.Unlock()
		s.buffer.Release()
		s.sample.Release()
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

func (v *IMFSourceReader) SetStreamSelection(index int64, selected bool) error {
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

func (v *IMFSourceReader) SetCurrentMediaType(index int64, mt *IMFMediaType) error {
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

func (v *IMFSourceReader) GetCurrentMediaType(index int64, mt **IMFMediaType) error {
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

func (v *IMFSourceReader) ReadSample(index, controlFlags int64, actualIndex *int64, streamFlags *int64, timestamp *int64, sample **IMFSample) error {
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

func (v *IMFMediaBuffer) Lock(buf **byte, length, capacity *int64) error {
	r, _, _ := syscall.SyscallN(
		v.VTable.Lock,
		uintptr(unsafe.Pointer(v)),
		uintptr(unsafe.Pointer(buf)),
		uintptr(unsafe.Pointer(length)),
		uintptr(unsafe.Pointer(capacity)),
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
	_mfplat      = windows.NewLazySystemDLL("Mfplat.dll")
	_shlwapi     = windows.NewLazySystemDLL("Shlwapi.dll")
	_mfreadwrite = windows.NewLazySystemDLL("Mfreadwrite.dll")

	_SHCreateMemStream = _shlwapi.NewProc("SHCreateMemStream")

	_MFStartup                    = _mfplat.NewProc("MFStartup")
	_MFShutdown                   = _mfplat.NewProc("MFShutdown")
	_MFCreateMediaType            = _mfplat.NewProc("MFCreateMediaType")
	_MFCreateAttributes           = _mfplat.NewProc("MFCreateAttributes")
	_MFCreateMFByteStreamOnStream = _mfplat.NewProc("MFCreateMFByteStreamOnStream")

	_MFCreateSourceReaderFromByteStream = _mfreadwrite.NewProc("MFCreateSourceReaderFromByteStream")
)

func MFStartup(version, flags uintptr) error {
	r, _, _ := _MFStartup.Call(version, flags)
	if r != S_OK {
		return MFErr{Code: r}
	}
	return nil
}

func MFShutdown() error {
	r, _, _ := _MFStartup.Call()
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
	return (*IStream)(unsafe.Pointer(r))
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
		_mfplat.Handle(),
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
