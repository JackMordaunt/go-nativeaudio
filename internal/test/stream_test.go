package test

import (
	"bytes"
	"io"
	"testing"
	"time"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

// TestStreamMatchesDecode ensures the incremental path produces exactly
// the same PCM as the buffered one. Any divergence here means the two
// code paths have drifted apart.
func TestStreamMatchesDecode(t *testing.T) {
	d := newDecoder(t)

	want, wantFormat, err := d.Decode(compressed)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	s, err := d.Stream(compressed)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer s.Close()

	// The format must be known before a single byte is read.
	if s.Format() != wantFormat {
		t.Errorf("stream format: want %+v, got %+v", wantFormat, s.Format())
	}

	got, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stream output differs from buffered decode: %d vs %d bytes", len(got), len(want))
	}
}

// TestStreamFileMatchesDecodeFile covers the file entry point, which on
// some backends pipes the decode rather than buffering it.
func TestStreamFileMatchesDecodeFile(t *testing.T) {
	d := newDecoder(t)

	want, wantFormat, err := d.DecodeFile("compressed.m4a")
	if err != nil {
		t.Fatalf("DecodeFile: %v", err)
	}

	s, err := d.StreamFile("compressed.m4a")
	if err != nil {
		t.Fatalf("StreamFile: %v", err)
	}
	defer s.Close()

	if s.Format() != wantFormat {
		t.Errorf("stream format: want %+v, got %+v", wantFormat, s.Format())
	}

	got, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stream output differs from buffered decode: %d vs %d bytes", len(got), len(want))
	}
}

// TestStreamSmallReads ensures the stream honours whatever buffer size
// the caller offers, rather than assuming it is handed a large one.
func TestStreamSmallReads(t *testing.T) {
	d := newDecoder(t)

	want, _, err := d.Decode(compressed)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	s, err := d.Stream(compressed)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer s.Close()

	var (
		got [][]byte
		buf = make([]byte, 7) // deliberately small and not sample-aligned
	)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			got = append(got, append([]byte(nil), buf[:n]...))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading stream: %v", err)
		}
	}
	if joined := bytes.Join(got, nil); !bytes.Equal(joined, want) {
		t.Fatalf("small reads produced %d bytes, want %d", len(joined), len(want))
	}
}

// TestStreamAbandonedEarly closes a stream without draining it. Backends
// hold decoder state and, where ffmpeg is used, a live subprocess, so
// this must not leak either.
func TestStreamAbandonedEarly(t *testing.T) {
	d := newDecoder(t)
	for i := 0; i < 5; i++ {
		s, err := d.Stream(compressed)
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		if _, err := io.CopyN(io.Discard, s, 1024); err != nil {
			t.Fatalf("partial read: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("closing abandoned stream: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("second close should be a no-op: %v", err)
		}
	}
}

// TestCloseWaitsForStreams ensures the decoder does not tear down
// platform state while a stream is still using it.
func TestCloseWaitsForStreams(t *testing.T) {
	d, err := nativeaudio.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s, err := d.Stream(compressed)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	closed := make(chan error, 1)
	go func() { closed <- d.Close() }()

	select {
	case err := <-closed:
		t.Fatalf("Close returned while a stream was open: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := s.Close(); err != nil {
		t.Fatalf("closing stream: %v", err)
	}

	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return after the stream was closed")
	}

	if _, err := d.Stream(compressed); err != nativeaudio.ErrClosed {
		t.Fatalf("stream after close: want ErrClosed, got %v", err)
	}
}

// TestFFmpegStream covers the piped ffmpeg decode directly, on every
// platform that has ffmpeg rather than only where it is the backend.
func TestFFmpegStream(t *testing.T) {
	requireTools(t, "ffmpeg", "ffprobe")

	want, wantFormat, err := nativeaudio.FFmpegLoad("compressed.m4a")
	if err != nil {
		t.Fatalf("FFmpegLoad: %v", err)
	}

	s, err := nativeaudio.FFmpegStream("compressed.m4a")
	if err != nil {
		t.Fatalf("FFmpegStream: %v", err)
	}
	defer s.Close()

	if s.Format() != wantFormat {
		t.Errorf("stream format: want %+v, got %+v", wantFormat, s.Format())
	}

	got, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("piped ffmpeg output differs from buffered: %d vs %d bytes", len(got), len(want))
	}
}

// TestFFmpegStreamAbandoned kills the ffmpeg process early. There is no
// portable way to inspect the process table, so the timeout is what
// catches a stream that fails to reap its child.
func TestFFmpegStreamAbandoned(t *testing.T) {
	requireTools(t, "ffmpeg", "ffprobe")
	for i := 0; i < 3; i++ {
		s, err := nativeaudio.FFmpegStream("compressed.m4a")
		if err != nil {
			t.Fatalf("FFmpegStream: %v", err)
		}
		if _, err := io.CopyN(io.Discard, s, 512); err != nil {
			t.Fatalf("partial read: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- s.Close() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("closing abandoned ffmpeg stream: %v", err)
			}
		case <-time.After(30 * time.Second):
			// Generous on purpose. This guards against a stream that never
			// reaps its child at all, not against a slow one: a loaded CI
			// runner took ten seconds just to tear the process down.
			t.Fatal("closing an abandoned ffmpeg stream hung")
		}
	}
}
