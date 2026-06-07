package audit

import (
	"fmt"
	"sync"
)

/*
WriterPool refcount-opens one Writer per file path so story and trader share a
single serialised queue without package-level globals.
*/
type WriterPool struct {
	mu      sync.Mutex
	entries map[string]*pooledWriter
}

type pooledWriter struct {
	writer *Writer
	refs   int
}

/*
NewWriterPool constructs an empty audit writer pool.
*/
func NewWriterPool() *WriterPool {
	return &WriterPool{
		entries: make(map[string]*pooledWriter),
	}
}

/*
OpenConfigured opens the writer for trading.audit.file when set.
*/
func (pool *WriterPool) OpenConfigured() (*Writer, error) {
	path := auditPath()

	if path == "" {
		return nil, nil
	}

	return pool.Open(path)
}

/*
Open returns a refcounted writer for path.
*/
func (pool *WriterPool) Open(path string) (*Writer, error) {
	if pool == nil {
		return nil, fmt.Errorf("audit: nil writer pool")
	}

	if path == "" {
		return nil, nil
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if entry := pool.entries[path]; entry != nil {
		entry.refs++

		return entry.writer, nil
	}

	writer, err := openWriter(path)

	if err != nil {
		return nil, err
	}

	writer.pool = pool
	pool.entries[path] = &pooledWriter{writer: writer, refs: 1}

	return writer, nil
}

func (pool *WriterPool) release(writer *Writer) error {
	if pool == nil || writer == nil {
		return nil
	}

	pool.mu.Lock()

	entry := pool.entries[writer.path]

	if entry == nil || entry.writer != writer {
		pool.mu.Unlock()

		return writer.closeDirect()
	}

	entry.refs--

	if entry.refs > 0 {
		pool.mu.Unlock()

		return writer.Err()
	}

	delete(pool.entries, writer.path)
	pool.mu.Unlock()

	return writer.closeDirect()
}
