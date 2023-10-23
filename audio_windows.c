#include "audio_windows.h"

FormatResult 
GetFormat(IMFMediaType * m_type);

// ErrorStr creates an error with the provided message as a char*.
Error*
ErrorStr(char *s)
{
        Error *err = calloc(1, sizeof(Error));
        err->Str = s;
        return err;
}

// ErrorWrap wraps an error with more context.
Error*
ErrorWrap(Error *err, char* s)
{
        Error *new = calloc(1, sizeof(Error));
        new->Str = s;
        new->Err = err;
        return new;
}

// ErrorWithCode attaches an error code to the error.
Error*
ErrorWithCode(Error *err, int code)
{
        err->Code = code;
        return err;
}

// ErrorFree deallocates the error and any wrapped errors. 
void 
ErrorFree(Error* err)
{
        Error *tmp = NULL;
        while (err != NULL) 
        {
                tmp = err;
                err = err->Err;
                free(tmp);
        }
}

// NewResult constructs a Result with the provided value and error.
// Usually one of the hose pointers will be NULL.
Result
NewResult(void* value, Error* err)
{
        return (Result){value, err};
}

// CharWiden converts a raw C string to a wide string used by Windows.
Result
CharWiden(char* str)
{
        int hr = 0;
        size_t length = 0;
        WCHAR *target = NULL;

        length = strlen(str);
        target = calloc(sizeof(WCHAR), length);

        if ((hr = MultiByteToWideChar(CP_ACP, 0, str, -1, target, length)) != 0)
        {
                return NewResult(NULL, ErrorWithCode(ErrorStr("converting C string to Windows wide string (WCHAR)"), hr));
        }

        return NewResult(target, NULL);
}

// BufferNew allocates a new buffer ready to use.
Buffer* 
BufferNew()
{
        Buffer *buffer = calloc(1, sizeof(Buffer));
        buffer->Data = calloc(BUFFER_DEFAULT_SIZE, 1);
        buffer->Len = 0;
        buffer->Cap = BUFFER_DEFAULT_SIZE;
        return buffer;
}

// BufferGrow grows the capacity by at least the provided amount. 
// Growth algorithm uses the double+1 strategy. 
void
BufferGrow(Buffer *buffer, int amount) 
{
        BYTE* tmp = NULL;
        int target = buffer->Cap;
        if (buffer->Cap > buffer->Len + amount) 
        {
                return;
        }
        // Expand the target capacity until we can fit the amount. 
        while (target < buffer->Len + amount)
        {
                target = target*2+1;
        }
        tmp = calloc(target, 1);
        memcpy(tmp, buffer->Data, buffer->Len);
        free(buffer->Data);
        buffer->Data = tmp;
        buffer->Cap = target;
}

// BufferWrite the data to the buffer, growing if necessary. 
void
BufferWrite(Buffer* buffer, int size, BYTE* data)
{       
        if (buffer->Cap < buffer->Len + size)
        {
                BufferGrow(buffer, size);
        }
        memcpy(buffer->Data+buffer->Len, data, size);
        buffer->Len += size;
}

// BufferFree frees the memory for the buffer. 
void
BufferFree(Buffer* buffer) 
{
        if (buffer == NULL) {
                return;
        }
        free(buffer->Data);
        free(buffer);
}


// RunMediaSession executes the media session until complete. This
// should output the sound.
// This is used in Play. 
HRESULT
RunMediaSession(IMFMediaSession* pSession) {

    HRESULT hr = S_OK;

    BOOL bSessionEvent = TRUE;

    while(bSessionEvent){

        HRESULT hrStatus = S_OK;
        IMFMediaEvent* pEvent = NULL;
        MediaEventType meType = MEUnknown;

        MF_TOPOSTATUS TopoStatus = MF_TOPOSTATUS_INVALID;

        hr = pSession->lpVtbl->GetEvent(pSession, 0, &pEvent);

        if(SUCCEEDED(hr)){
            hr = pEvent->lpVtbl->GetStatus(pEvent, &hrStatus);
        }

        if(SUCCEEDED(hr)){
            hr = pEvent->lpVtbl->GetType(pEvent, &meType);
        }

        if(SUCCEEDED(hr) && SUCCEEDED(hrStatus)){

            switch(meType){

                case MESessionTopologySet:
                    break;

                case MESessionTopologyStatus:

                    hr = pEvent->lpVtbl->GetUINT32(pEvent, &MF_EVENT_TOPOLOGY_STATUS, (UINT32*)&TopoStatus);

                    if(SUCCEEDED(hr)){

                        switch(TopoStatus){

                            case MF_TOPOSTATUS_READY:
                            {
                                PROPVARIANT varStartPosition;
                                PropVariantInit(&varStartPosition);
                                hr = pSession->lpVtbl->Start(pSession, NULL, &varStartPosition);
                                PropVariantClear(&varStartPosition);
                            }
                            break;

                            case MF_TOPOSTATUS_STARTED_SOURCE:
                                break;

                            case MF_TOPOSTATUS_ENDED:
                                break;

                            default:
                                break;
                        }
                    }
                    break;

                case MESessionStarted:
                    break;

                case MESessionEnded:
                    hr = pSession->lpVtbl->Stop(pSession);
                    break;

                case MESessionStopped:
                    hr = pSession->lpVtbl->Close(pSession);
                    break;

                case MESessionClosed:
                    bSessionEvent = FALSE;
                    break;

                case MESessionNotifyPresentationTime:
                    break;

                case MESessionCapabilitiesChanged:
                    break;

                case MEEndOfPresentation:
                    break;

                default:
                    break;
            }

            if(FAILED(hr) || FAILED(hrStatus)){
                bSessionEvent = FALSE;
            }
        }
    }

    return hr;
}

// AddOutputNode to the topology.
// This is used in Play. 
HRESULT
AddOutputNode(
    IMFTopology *pTopology,     // Topology.
    IMFStreamSink *pStreamSink, // Stream sink.
    IMFTopologyNode **ppNode   // Receives the node pointer.
)
{
    IMFTopologyNode *pNode = NULL;
    HRESULT hr = S_OK;

        // Create the node.
        hr = MFCreateTopologyNode(MF_TOPOLOGY_OUTPUT_NODE, &pNode);

        if (hr != S_OK)
        {
                return hr;
        }

        // Set the object pointer.
        if (SUCCEEDED(hr))
        {
                hr = pNode->lpVtbl->SetObject(pNode, (IUnknown *)pStreamSink);
        }

        if (hr != S_OK)
        {
            return hr;
        }

        // Add the node to the topology.
        if (SUCCEEDED(hr))
        {
                hr = pTopology->lpVtbl->AddNode(pTopology, pNode);
        }

        if (hr != S_OK)
        {
                return hr;
        }

        if (SUCCEEDED(hr))
        {
                hr = pNode->lpVtbl->SetUINT32(pNode, &MF_TOPONODE_NOSHUTDOWN_ON_REMOVE, TRUE);
        }

        if (hr != S_OK)
        {
                return hr;
        }

        // Return the pointer to the caller.
        if (SUCCEEDED(hr))
        {
                *ppNode = pNode;
                (*ppNode)->lpVtbl->AddRef(*ppNode);
        }

        return hr;
}

// Add a source node to a topology.
// This is used in Play. 
HRESULT
AddSourceNode(
    IMFTopology *pTopology,           // Topology.
    IMFMediaSource *pSource,          // Media source.
    IMFPresentationDescriptor *pPD,   // Presentation descriptor.
    IMFStreamDescriptor *pSD,         // Stream descriptor.
    IMFTopologyNode **ppNode         // Receives the node pointer.
)
{
        IMFTopologyNode *pNode = NULL;

        // Create the node.
        HRESULT hr = MFCreateTopologyNode(MF_TOPOLOGY_SOURCESTREAM_NODE, &pNode);
        if (FAILED(hr))
        {
                goto done;
        }

        // Set the attributes.
        hr = pNode->lpVtbl->SetUnknown(pNode, &MF_TOPONODE_SOURCE, (IUnknown *)pSource);
        if (FAILED(hr))
        {
                goto done;
        }

        hr = pNode->lpVtbl->SetUnknown(pNode, &MF_TOPONODE_PRESENTATION_DESCRIPTOR, (IUnknown *)pPD);
        if (FAILED(hr))
        {
                goto done;
        }

        hr = pNode->lpVtbl->SetUnknown(pNode, &MF_TOPONODE_STREAM_DESCRIPTOR, (IUnknown *)pSD);
        if (FAILED(hr))
        {
                goto done;
        }

        // Add the node to the topology.
        hr = pTopology->lpVtbl->AddNode(pTopology, pNode);
        if (FAILED(hr))
        {
                goto done;
        }

        // Return the pointer to the caller.
        *ppNode = pNode;
        (*ppNode)->lpVtbl->AddRef(*ppNode);

done:
    return hr;
}

//-------------------------------------------------------------------
// ConfigureAudioStream
//
// Selects an audio stream from the source file, and configures the
// stream to deliver decoded PCM audio.
//
// We can use the source reader to load all PCM samples in a loop.
//
//  hr = ConfigureAudioStream(pReader, &pAudioType);
//-------------------------------------------------------------------
HRESULT
ConfigureAudioStream(
    IMFSourceReader *pReader,   // Pointer to the source reader.
    IMFMediaType **pUncompressedAudioType,
    IMFMediaType **pPartialType,
    IMFMediaType **ppPCMAudio   // Receives the audio format.
)
{
    // Select the first audio stream, and deselect all other streams.
    HRESULT hr = pReader->lpVtbl->SetStreamSelection(pReader,
        (DWORD)MF_SOURCE_READER_ALL_STREAMS, FALSE);

    if (SUCCEEDED(hr))
    {
        hr = pReader->lpVtbl->SetStreamSelection(pReader,
            (DWORD)MF_SOURCE_READER_FIRST_AUDIO_STREAM, TRUE);
    }

    // Create a partial media type that specifies uncompressed PCM audio.
    hr = MFCreateMediaType(pPartialType);

    if (SUCCEEDED(hr))
    {
        hr = (*pPartialType)->lpVtbl->SetGUID((*pPartialType), &MF_MT_MAJOR_TYPE, &MFMediaType_Audio);
    }

    if (SUCCEEDED(hr))
    {
        hr = (*pPartialType)->lpVtbl->SetGUID((*pPartialType), &MF_MT_SUBTYPE, &MFAudioFormat_PCM);
    }

    // Set this type on the source reader. The source reader will
    // load the necessary decoder.
    if (SUCCEEDED(hr))
    {
        hr = pReader->lpVtbl->SetCurrentMediaType(pReader,
            (DWORD)MF_SOURCE_READER_FIRST_AUDIO_STREAM,
            NULL, (*pPartialType));
    }

    // Get the complete uncompressed format.
    if (SUCCEEDED(hr))
    {
        hr = pReader->lpVtbl->GetCurrentMediaType(pReader,
            (DWORD)MF_SOURCE_READER_FIRST_AUDIO_STREAM,
            pUncompressedAudioType);
    }

    // Ensure the stream is selected.
    if (SUCCEEDED(hr))
    {
        hr = pReader->lpVtbl->SetStreamSelection(pReader,
            (DWORD)MF_SOURCE_READER_FIRST_AUDIO_STREAM,
            TRUE);
    }

    // Return the PCM format to the caller.
    if (SUCCEEDED(hr))
    {
        *ppPCMAudio = (*pUncompressedAudioType);
        (*ppPCMAudio)->lpVtbl->AddRef(*ppPCMAudio);
    }

    return hr;
}

// Setup the source reader for the audio file at the given path. 
// IMFSourceReader* is returned as result value. 
Result
NewSourceReaderForFile(char* path) 
{
        Error *err = NULL; // Dyanmic error. 
        HRESULT hr = S_OK; // Windows return code.

        WCHAR           *audio_file_path = NULL; // Path to audio file.

        // reader is returned to caller. 
        IMFSourceReader *reader          = NULL; // Object to stream bytes from.

        // Since Windows uses wide strings we need to convert the char*
        // to such a format.
        Result r = CharWiden(path);

        if (r.Err != NULL)
        {
                err = ErrorWithCode(ErrorWrap(r.Err, "converting path string"), hr);
                goto done;
        }

        audio_file_path = (WCHAR*)r.Value;

        hr = MFCreateSourceReaderFromURL(audio_file_path, NULL, &reader);
        
        if (FAILED(hr))
        {
                err = ErrorWithCode(ErrorStr("creating source reader"), hr);
                goto done;
        }

done:

        free(audio_file_path);

        if (err != NULL) 
        {
                r.Err = err;
        }

        if (reader != NULL) 
        {
                r.Value = reader;
        }
        
        return r;
}


// GetFormat reads format meta data from a source reader. 
FormatResult 
GetFormat(IMFMediaType * m_type) 
{
        HRESULT hr = S_OK;
        FormatResult r = {
                .Format = {
                        .SampleRate = 0,
                        .BitDepth = 0,
                        .Channels = 0
                },
                .Err = NULL,
        };
        
        UINT32 num_channels = 0;

        hr = m_type->lpVtbl->GetUINT32(m_type, &MF_MT_AUDIO_NUM_CHANNELS, &num_channels);

        if (FAILED(hr))
        {
                r.Err = ErrorWithCode(ErrorStr("getting num channels"), hr);
                goto done;
        }

        UINT32 sample_rate = 0;

        hr = m_type->lpVtbl->GetUINT32(m_type, &MF_MT_AUDIO_SAMPLES_PER_SECOND, &sample_rate);

        if (FAILED(hr))
        {
                r.Err = ErrorWithCode(ErrorStr("getting num channels"), hr);
                goto done;
        }
        

        UINT32 bits_per_sample = 0;

        hr = m_type->lpVtbl->GetUINT32(m_type, &MF_MT_AUDIO_BITS_PER_SAMPLE, &bits_per_sample);

        if (FAILED(hr))
        {
                r.Err = ErrorWithCode(ErrorStr("getting num channels"), hr);
                goto done;
        }
        
        GUID sub_type;

        hr = m_type->lpVtbl->GetGUID(m_type, &MF_MT_SUBTYPE, &sub_type);

        if (FAILED(hr))
        {
                r.Err = ErrorWithCode(ErrorStr("getting sub type"), hr);
                goto done;
        }
        
done:

        r.Format = (Format){
                .SampleRate = sample_rate,
                .Channels = num_channels,
                .BitDepth = bits_per_sample / 8,
        };

        return r;
}


// decode buffers the decoded PCM s16le data and returns it via out.
Error*
decode(IMFSourceReader * reader, Buffer * out)
{
        assert(reader);
        
        IMFMediaBuffer *bufferReader = NULL; // buffer object containing the raw buffer.
        IMFSample *pSample = NULL;           // sample object containing on or more streams.
        BYTE *chunk = NULL;                  // pointer to start of chunk.
        
        LONGLONG prev_time_stamp = -1; 
        LONGLONG time_stamp = 0;
        DWORD cbBuffer = 0;                  // size of chunk.
        HRESULT hr = S_OK;
        Error *err = NULL;

        // Stream all the data into a byte buffer.

        // NOTE(jfm): we can create a streaming api by extracting this loop
        // to the Go side, and implement something like an io.Reader. 
        // However this api currently reads the entire thing and passes
        // it all back to Go at once. 
        while (1) {
                DWORD dwFlags = 0;
                
                if (pSample != NULL)
                {
                        pSample->lpVtbl->RemoveAllBuffers(pSample);
                        pSample->lpVtbl->Release(pSample);
                }
                
                // Read the next sample.
                hr = reader->lpVtbl->ReadSample(
                        reader,
                        (DWORD)MF_SOURCE_READER_FIRST_AUDIO_STREAM,
                        0,
                        NULL,
                        &dwFlags,
                        &time_stamp,
                        &pSample
                );

                // NOTE(jfm): Avoid chunks that we have already seen. 
                //
                // For some reason, ReadSample can produce more than
                // one sample at time stamp "0". 
                //
                // Emitting all of them produces both larger files and
                // audio artefacts. 
                if (time_stamp == prev_time_stamp) 
                {
                        continue;
                }

                prev_time_stamp = time_stamp;

                if (FAILED(hr))
                {
                        err = ErrorWithCode(ErrorStr("reading sample"), hr);
                        goto done;
                }

                if (dwFlags & MF_SOURCE_READERF_CURRENTMEDIATYPECHANGED)
                {
                        break;
                }
                if (dwFlags & MF_SOURCE_READERF_ENDOFSTREAM)
                {
                        break;
                }

                if (pSample == NULL)
                {
                        continue;
                }

                if (bufferReader != NULL) {
                        bufferReader->lpVtbl->Release(bufferReader);
                }

                // Get a pointer to the buffer object.
                hr = pSample->lpVtbl->ConvertToContiguousBuffer(pSample, &bufferReader);

                if (FAILED(hr))
                {
                        err = ErrorWithCode(ErrorStr("converting to contiguous buffer"), hr);
                        goto done;
                }

                // Get read/write access to the next chunk of audio data.
                hr = bufferReader->lpVtbl->Lock(bufferReader, &chunk, NULL, &cbBuffer);

                if (FAILED(hr))
                {
                        err = ErrorWithCode(ErrorStr("locking buffer"), hr);
                        goto done;
                }
        
                BufferWrite(out, cbBuffer, chunk);

                // Unlock the reader that we just copied from.
                hr = bufferReader->lpVtbl->Unlock(bufferReader);

                if (FAILED(hr))
                {
                        err = ErrorWithCode(ErrorStr("unlocking buffer"), hr);
                        goto done;
                }

                chunk = NULL;                
        }

done:

        if (pSample != NULL)
        {
                pSample->lpVtbl->RemoveAllBuffers(pSample);
                pSample->lpVtbl->Release(pSample);
        }

        if (bufferReader != NULL) 
        {
                bufferReader->lpVtbl->Release(bufferReader);
        }

        return err;
}

// Decode the compresed audio data into uncompressed s16le PCM. 
//
// We get a bit lucky here because Media Foundation defaults to that
// PCM format when it auto-inits the AAC decoder. 
// 
// For different input formats (other than AAC) the PCM format may not
// be guaranteed. 
DecodeResult
Decode(BYTE* compressed, UINT size)
{
        assert(compressed);

        HRESULT hr = S_OK;
        DecodeResult r = {
                .Uncompressed = NULL,
                .Format = { .Channels = 0, .SampleRate = 0, .BitDepth = 0},
                .Err = NULL,
        };

        // stream is the type required by Media Foundation. 
        // We can get one of these by wrapping a COM IStream. 
        IMFByteStream * stream = NULL;
        // mem_stream is a plain COM IStream that streams from an in-memory
        // buffer. 
        IStream * mem_stream = SHCreateMemStream(compressed, size);

        IMFMediaType * m_type = NULL;
        IMFMediaType * pUncompressedAudioType = NULL;
        IMFMediaType * pPartialType = NULL;
        Buffer * buffer = BufferNew();

        hr = MFCreateMFByteStreamOnStream(mem_stream, &stream);

        if (FAILED(hr)) 
        {
                r.Err = ErrorWithCode(ErrorStr("creating byte stream"), hr);
                goto done;
        }

        IMFSourceReader *reader = NULL;

        hr = MFCreateSourceReaderFromByteStream(stream, NULL, &reader);

        if (FAILED(hr))
        {
                r.Err = ErrorWithCode(ErrorStr("creating source reader from byte stream"), hr);
                goto done;
        }


        hr = ConfigureAudioStream(reader, &pUncompressedAudioType, &pPartialType, &m_type);

        if (FAILED(hr))
        {
                r.Err = ErrorWithCode(ErrorStr("configuring audio stream"), hr);
                goto done;
        }

        FormatResult fr = GetFormat(m_type);

        if (fr.Err != NULL)
        {
                r.Err = ErrorWrap(fr.Err, "getting format");
                goto done;
        }

        r.Err = decode(reader, buffer);

        if (r.Err != NULL) 
        {
                r.Err = ErrorWrap(r.Err, "decode minor");
        }

        assert(buffer);

done:
        if (pUncompressedAudioType != NULL) {
                pUncompressedAudioType->lpVtbl->Release(pUncompressedAudioType);
        }
        
        if (pPartialType != NULL) {
                pPartialType->lpVtbl->Release(pPartialType);
        }

        if (m_type != NULL)
        {
                m_type->lpVtbl->Release(m_type);
        }

        if (stream != NULL)
        {
                stream->lpVtbl->Release(stream);
        }

        if (mem_stream != NULL) {
                mem_stream->lpVtbl->Release(mem_stream);
        }

        if (reader != NULL)
        {
                reader->lpVtbl->Release(reader);
        }

        if (buffer != NULL)
        {
                r.Uncompressed = buffer;
        }

        r.Format = fr.Format;

        return r;
}

// Load decodes the file at path and returns raw PCM s16le with the 
// given format required for correct playback. 
DecodeResult
Load(char* path)
{
        IMFSourceReader *reader = NULL;         // Object to stream bytes from.
        Buffer *buffer = NULL;                  // Buffer to accumulate decoded PCM and return to Go.
        Error *err = NULL;                      // Dyanmic error. 
        HRESULT hr = S_OK;                      // Windows return code.
        FormatResult fr = {
                .Err = NULL,
                .Format = {
                        .BitDepth = 0,
                        .SampleRate = 0,
                        .Channels = 0,
                }
        };
        DecodeResult dr = {
                .Err = NULL,
                .Uncompressed = NULL,
                .Format = {
                        .BitDepth = 0,
                        .SampleRate = 0,
                        .Channels = 0,
                }
        };
        
        IMFMediaType * m_type = NULL;
        IMFMediaType * pUncompressedAudioType = NULL;
        IMFMediaType * pPartialType = NULL;

        Result r = NewSourceReaderForFile(path);
        
        if (r.Err != NULL)
        {
                dr.Err = ErrorWrap(r.Err, "setting up source reader for audio file");
                goto done;
        }

        reader = (IMFSourceReader*)(r.Value);


        hr = ConfigureAudioStream(reader, &pUncompressedAudioType, &pPartialType, &m_type);

        if (FAILED(hr))
        {
                err = ErrorWithCode(ErrorStr("configuring audio stream"), hr);
                goto done;
        }  

        fr = GetFormat(m_type);

        if (fr.Err != NULL)
        {
                dr.Err = ErrorWrap(fr.Err, "getting format");
                goto done;
        }

        assert(fr.Format.BitDepth != 0);
        assert(fr.Format.BitDepth <= 2);
        assert(fr.Format.SampleRate != 0);
        assert(fr.Format.Channels != 0);
        assert(fr.Format.Channels <= 2);

        // Heap allocated buffer to accumulate the audio data. 
        // NOTE(jfm): Free from cgo side with BufferFree().
        buffer = BufferNew(); 

        err = decode(reader, buffer);
        
        if (err != NULL) 
        {
                dr.Err = ErrorWrap(err, "decode minor");
                goto done;
        }

done:

        if (pUncompressedAudioType != NULL) {
                pUncompressedAudioType->lpVtbl->Release(pUncompressedAudioType);
        }
        
        if (pPartialType != NULL) {
                pPartialType->lpVtbl->Release(pPartialType);
        }
        
        if (m_type != NULL)
        {
                m_type->lpVtbl->Release(m_type);
        }

        if (reader != NULL) 
        {
                reader->lpVtbl->Release(reader);
        }

        dr.Uncompressed = buffer;
        dr.Format = fr.Format;

        return dr;
}

// Play the audio file at the given path directly to the speakers. 
Error*
Play(char* path)
{
        Error *err = NULL;
        Result r; 
        HRESULT hr;

        // We have to build a wide string from the C string for the path.
        // NOTE(jfm): size cannot exceed 256 without dynamic allocation.
        WCHAR    *audio_file_path = NULL;

        // session is the top-level object wherein all stream 
        // processing occurs.
        IMFMediaSession *session;
        // resolver can resolve an abitrary byte source.
        // In our case this will be a plain audio file.  
        IMFSourceResolver *resolver;

        // obj_type describes the source: media source or byte source.  
        MF_OBJECT_TYPE obj_type;
        // obj is the true source object. 
        IUnknown *obj;
        // src is the object casted to it's concrete type. 
        IMFMediaSource *src = NULL;
        // desc provides meta data about the playback. 
        IMFPresentationDescriptor *desc;
        
        // Stream selection: we assume exactly 1 stream of AAC audio, 
        // however this code may in fact be too brittle. 
        IMFStreamDescriptor *stream_desc; // info about the stream.
        BOOL fSelected = FALSE;           // if a stream exists for the index.
        DWORD stream_count = 0;           // number of streams found.

        // topology configures a graph of stream processing. The only
        // processing we want is to decode AAC LC audio.
        IMFTopology *topology;

        // activate is an object that can initialize itself. 
        // This wraps the audio decoder. 
        IMFActivate *activate;

        // source and output nodes: the only two nodes in our topology
        // graph. 
        IMFTopologyNode     *pSourceNode = NULL;
        IMFTopologyNode     *pOutputNode = NULL;
        
        // Since Windows uses wide strings we need to convert the char*
        // to such a format.
        r = CharWiden(path);

        if (r.Err != NULL)
        {
                err = ErrorWrap(r.Err, "converting path string");
                goto done;
        }

        audio_file_path = (WCHAR*)r.Value;

        // Create a media session which orchestrates the media processing
        // graph.
        if ((hr = MFCreateMediaSession(NULL, &session)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("creating media session"), hr);
                goto done;
        }
        
        // Create a source resolver. This object can open files and urls.
        if ((hr = MFCreateSourceResolver(&resolver)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("creating source resolver"), hr);
                goto done;
        }

        // Create a "media source" from the sound file.
        // Perhaps a bytestream would also work?
        if ((hr = resolver->lpVtbl->CreateObjectFromURL(
                resolver,
                audio_file_path,
                MF_RESOLUTION_MEDIASOURCE|MF_RESOLUTION_READ,
                NULL,
                &obj_type,
                &obj
        )) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("creating object from url"), hr);
                goto done;
        }
        
        if (obj_type != MF_OBJECT_MEDIASOURCE)
        {
                err = ErrorWithCode(ErrorStr("not a media source"), hr);
                goto done;
        }

        // We know it's a media source so we can do the cast safely. 
        src = (IMFMediaSource*)obj;

        // Create a presentation descriptor.
        // This object describes the media source, e.g. what it's audio
        // and video streams are.
        if ((hr = src->lpVtbl->CreatePresentationDescriptor(src, &desc)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("creating presentation descriptor"), hr);
                goto done;
        }
        
        // Get information about the audio stream. We pull the count
        // and validate it, but we assume there's only one stream:
        // AAC-LC audio.
        if ((hr = desc->lpVtbl->GetStreamDescriptorCount(desc, &stream_count)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("getting stream descriptor count"), hr);
                goto done;
        }
        
        if (stream_count != 1)
        {
                err = ErrorWithCode(ErrorStr("expected exactly one stream"), hr);
                goto done;

        }

        if ((hr = desc->lpVtbl->GetStreamDescriptorByIndex(desc, 0, &fSelected, &stream_desc)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("getting stream descriptor by index"), hr);
                goto done;
        }
        
        if (!fSelected)
        {
                err = ErrorWithCode(ErrorStr("stream was not selected"), hr);
                goto done;
        }

        if ((hr = MFCreateAudioRendererActivate(&activate)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("creating audio renderer activate"), hr);
                goto done;
        }

        if ((hr = MFCreateTopology(&topology)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("creating topology"), hr);
                goto done;
        }

        if ((hr = AddSourceNode(topology, src, desc, stream_desc, &pSourceNode)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("adding source node"), hr);
                goto done;
        }
        
        if ((hr = AddOutputNode(topology, (IMFStreamSink *)activate, &pOutputNode)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("adding output node"), hr);
                goto done;
        }

        if ((hr = pSourceNode->lpVtbl->ConnectOutput(pSourceNode, 0, pOutputNode, 0)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("connect output"), hr);
                goto done;
        }
        
        if ((hr = session->lpVtbl->SetTopology(session, MFSESSION_SETTOPOLOGY_IMMEDIATE, topology)) != S_OK)
        {
                err = ErrorWithCode(ErrorStr("setting topology on session: %ld\n"), hr);
                goto done;
        }
        
        hr = RunMediaSession(session);
        
        if (hr != S_OK)
        {
                err = ErrorWithCode(ErrorStr("running media session"), hr);
                goto done;
        }

done:
        free(audio_file_path);
        if (pSourceNode != NULL) 
        {
                pSourceNode->lpVtbl->Release(pSourceNode);
        }
        if (pOutputNode != NULL) 
        {
                pOutputNode->lpVtbl->Release(pOutputNode);
        }
        if (activate != NULL) 
        {
                activate->lpVtbl->Release(activate);
        }
        if (topology != NULL) 
        {
                topology->lpVtbl->Release(topology);
        }
        if (stream_desc != NULL) 
        {
                stream_desc->lpVtbl->Release(stream_desc);
        }
        if (desc != NULL) 
        {
                desc->lpVtbl->Release(desc);
        }
        if (src != NULL) 
        {
                src->lpVtbl->Release(src);
        }
        if (obj != NULL) 
        {
                obj->lpVtbl->Release(obj);
        }
        if (resolver != NULL) 
        {
                resolver->lpVtbl->Release(resolver);
        }
        if (session != NULL) 
        {
                session->lpVtbl->Release(session);
        }
        return err;
}

Error* 
StartMediaFramework()
{
        
        Error * err = NULL;
        HRESULT hr = S_OK;

        hr = MFStartup(MF_VERSION, MFSTARTUP_LITE);

        if (FAILED(hr))
        {
                err = ErrorWithCode(ErrorStr("initializing media foundation"), hr);
                goto done;
        }

done: 
        return err;
}

Error* 
EndMediaFramework()
{
        Error * err = NULL;
        HRESULT hr = S_OK;

        hr = MFShutdown();

        if (FAILED(hr)) 
        {
                // Capture the shutdown error only if we didn't already encounter one. 
                if (err == NULL) 
                {
                        err = ErrorWithCode(ErrorStr("shutting down media foundation"), hr);
                }
        }

done:
        return err;
}