package internal

import "io"

// TriggerReader invokes a callback when EOF is reached.
type TriggerReader struct {
	r  io.Reader
	cb func()
}

func NewTriggerReader(src io.Reader, cb func()) *TriggerReader {
	return &TriggerReader{r: src, cb: cb}
}

func (t *TriggerReader) Read(p []byte) (n int, err error) {
	n, err = t.r.Read(p)
	if err == io.EOF {
		t.Callback()
	}
	return n, err
}

func (t *TriggerReader) Callback() {
	if t.cb != nil {
		t.cb()
	}
}
