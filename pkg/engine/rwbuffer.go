package engine

import (
	"io"
	"sync"
)

type BufWriter struct {
	sync.RWMutex

	data   []byte
	cond   *sync.Cond
	closed bool
}

type BufReader struct {
	writer *BufWriter
	offs   int
}

func NewBufWriter() *BufWriter {
	writer := &BufWriter{}
	writer.cond = sync.NewCond(writer)
	return writer
}

func (w *BufWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	data := make([]byte, len(p))
	copy(data, p)
	w.Lock()
	w.data = append(w.data, data...)
	w.Unlock()
	w.cond.Broadcast()
	return len(p), nil
}

func (w *BufWriter) Close() {
	w.Lock()
	w.closed = true
	w.Unlock()
	w.cond.Broadcast()
}

func NewBufReader(writer *BufWriter) io.Reader {
	reader := &BufReader{
		writer: writer,
	}
	return reader
}

func (r *BufReader) Read(p []byte) (int, error) {
	r.writer.cond.L.Lock()
	defer r.writer.cond.L.Unlock()

	if r.offs < len(r.writer.data) {
		size := min(len(r.writer.data)-r.offs, len(p))
		newoffs := r.offs + size
		copy(p, r.writer.data[r.offs:newoffs])
		r.offs = newoffs
		return size, nil
	}

	if r.writer.closed {
		return 0, io.EOF
	}

	r.writer.cond.Wait()

	return 0, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
