package capture

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// body and socket forward network I/O while copying accepted bytes into capture.
type body struct {
	source   io.ReadCloser
	stream   *stream
	length   int64
	read     atomic.Int64
	keepOpen bool
	eof      atomic.Bool
	observe  func(error)
}

func (b *body) Read(p []byte) (int, error) {
	n, err := b.source.Read(p)
	b.observe(err)
	if err == io.EOF {
		b.eof.Store(true)
	}
	if n > 0 {
		b.stream.write(p[:n], time.Now())
		b.read.Add(int64(n))
	}
	if !b.keepOpen {
		if err == io.EOF || (err == nil && b.length >= 0 && b.read.Load() == b.length) {
			b.stream.finish(nil)
		} else if err != nil {
			b.stream.finish(err)
		}
	} else if err != nil && err != io.EOF {
		b.stream.finish(err)
	}
	return n, err
}

func (b *body) Close() error {
	if !b.keepOpen {
		if b.eof.Load() || (b.length >= 0 && b.read.Load() == b.length) {
			b.stream.finish(nil)
		} else {
			b.observe(io.ErrUnexpectedEOF)
			b.stream.finish(io.ErrUnexpectedEOF)
		}
	}
	return b.source.Close()
}

type socket struct {
	source            io.ReadWriteCloser
	request, response *stream
	observe           func(error)
	readMu, writeMu   sync.Mutex
	closing           atomic.Bool
	closeOnce         sync.Once
	closeErr          error
}

func (s *socket) Read(p []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	if s.closing.Load() {
		return 0, net.ErrClosed
	}
	n, err := s.source.Read(p)
	s.observeNetworkError(err)
	if n > 0 {
		s.response.write(p[:n], time.Now())
	}
	if err == io.EOF {
		s.response.finish(nil)
	} else if err != nil && !s.closedLocally(err) {
		s.response.finish(err)
	}
	return n, err
}

func (s *socket) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.closing.Load() {
		return 0, net.ErrClosed
	}
	n, err := s.source.Write(p)
	if n < len(p) && err == nil {
		err = io.ErrShortWrite
	}
	s.observeNetworkError(err)
	if n > 0 {
		s.request.write(p[:n], time.Now())
	}
	if err != nil && !s.closedLocally(err) {
		s.request.finish(err)
	}
	return n, err
}

func (s *socket) Close() error {
	s.closeOnce.Do(func() {
		s.closing.Store(true)
		// Closing the network first releases blocked I/O. Do not close the
		// capture streams yet: an operation may still be enqueuing bytes that
		// the upstream already accepted. Cancellation/shutdown can abort that
		// wait through the streams, without needing these direction locks.
		s.closeErr = s.source.Close()
		s.observeNetworkError(s.closeErr)
		s.writeMu.Lock()
		s.request.finish(nil)
		s.writeMu.Unlock()
		s.readMu.Lock()
		s.response.finish(nil)
		s.readMu.Unlock()
	})
	return s.closeErr
}

func (s *socket) closedLocally(err error) bool {
	return s.closing.Load() && (errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe))
}

func (s *socket) observeNetworkError(err error) {
	if !s.closedLocally(err) {
		s.observe(err)
	}
}
