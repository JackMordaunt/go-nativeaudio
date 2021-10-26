#include <crtdbg.h>
#include <stdlib.h>
#include <stdio.h>
#include <Windows.h>
#include <winbase.h>
#include <combaseapi.h>
#include <mfapi.h>
#include <Mfidl.h>
#include <mferror.h>
#include <initguid.h>
#include <wmcodecdsp.h>
#include <mmdeviceapi.h>
#include <mfreadwrite.h>
#include <shlwapi.h>
#include <assert.h>
#include <stdint.h>

// TODO: native volume (https://docs.microsoft.com/en-us/windows/win32/api/mfidl/nn-mfidl-imfaudiostreamvolume)

// Error declares an error return containing a message and possibly
// wrapping another error.
//
// Errors are dynamically allocated and are designed to provide immediate
// feedback with a user facing description.
//
// Use the wrapped error code to get the raw code for manual lookup.
typedef struct Error
{
        int Code;          // Underlying error code from traditional C calls.
        struct Error* Err; // Wrapped error, if any.
        char* Str;         // String description of error.
} Error;

// Result captures a generic value return along side a possible error.
// Check error before accessing value. Caller must know what type the
// value can be.
typedef struct Result
{
        void* Value;
        Error* Err;
} Result;


// Format describes uncompressed PCM necessary for correct playback. 
typedef struct Format
{
        uint32_t SampleRate;
        uint32_t Channels;
        uint32_t BitDepth; 
} Format;


// Buffer describes a dynamic byte buffer with a length, capacity and 
// a pointer to the first element. 
typedef struct Buffer
{
        int Len;    // Len is the currently used region of the buffer. 
        int Cap;    // Capacity is the total allocated memory. 
        BYTE* Data; // Data is the pointer to the first byte. 
} Buffer; 



// FormatResult captures the result of decoding an audio buffer. 
typedef struct FormatResult
{
        Format Format;
        Error* Err;
} FormatResult;


// DecodeResult captures the result of decoding an audio buffer. 
typedef struct DecodeResult
{
        Buffer* Uncompressed;
        Format Format;
        Error* Err;
} DecodeResult;

// BufferFree deallocates the memory for a buffer, including the pointer
// to it and it's pointer to the raw data. 
void BufferFree(Buffer*);

// ErrorFree deallocates the memory for an error and all wrapped errors. 
void ErrorFree(Error*);

// Load the decoded PCM data from the given file.
//
// Load is implemented over the top of Windows Media Foundation and
// what a wild ride that is.
//
// https://docs.microsoft.com/en-us/windows/win32/medfound/about-the-media-foundation-sdk
DecodeResult Load(char* path);

// Play an audio file at the given file path.
//
// Play is implemented over the top of Windows Media Foundation and
// what a wild ride that is.
//
// https://docs.microsoft.com/en-us/windows/win32/medfound/about-the-media-foundation-sdk
Error* Play(char *path);

// Decode a buffer of compressed audio using Windows Media Foundation. 
DecodeResult Decode(BYTE* compressed, UINT size);

// Stub to compile against mingw64 which apparently does not include 
// this function in it's header file.  
HRESULT MFCreateMFByteStreamOnStream(
        IStream       *pStream,
        IMFByteStream **ppByteStream
);

// StartMediaFramewok initializes the media framework ready to decode
// and playback audio.  
Error*
StartMediaFramework();

// EndMediaFramewok shuts down the media framework. Decoding and playback
// will not work hence forth.  
Error*
EndMediaFramework();