package capture

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const chunkSize = 32 << 10
const streamChunks = 8 // 256 KiB per direction, including the consumer's chunk.

// Shared by all recorders, including overlapping service runs during shutdown.
var captureBudget = make(chan struct{}, (32<<20)/chunkSize)

type chunk struct {
	data []byte
	at   time.Time
}

// stream owns copied chunks until its sole reader consumes them. Both the
// per-stream slots and process budget are acquired BEFORE allocating bytes.
type stream struct {
	mu         sync.Mutex
	closed     bool
	err        error
	done       chan struct{}
	queue      chan chunk
	slots      chan struct{}
	budget     chan struct{}
	current    chunk
	at         time.Time // reader-owned observation time, never the disk processing time
	waitNS     atomic.Int64
	stopCancel func() bool
}

func newStream(ctx context.Context, budget chan struct{}) *stream {
	s := &stream{done: make(chan struct{}), queue: make(chan chunk, streamChunks), slots: make(chan struct{}, streamChunks), budget: budget}
	s.stopCancel = context.AfterFunc(ctx, func() { s.finish(ctx.Err()) })
	return s
}

func (s *stream) acquire(slots chan struct{}) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case slots <- struct{}{}:
		return true
	default:
		start := time.Now()
		defer func() { s.waitNS.Add(int64(time.Since(start))) }()
		select {
		case slots <- struct{}{}:
			return true
		case <-s.done:
			return false
		}
	}
}

func (s *stream) write(p []byte, at time.Time) {
	for len(p) > 0 {
		if !s.acquire(s.slots) {
			return
		}
		if !s.acquire(s.budget) {
			<-s.slots
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			<-s.slots
			<-s.budget
			return
		}
		n := min(len(p), chunkSize)
		data := append([]byte(nil), p[:n]...)
		s.queue <- chunk{data, at} // A reserved local slot guarantees room.
		s.mu.Unlock()
		p = p[n:]
	}
}

func (s *stream) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.err = err
		close(s.done)
	}
}

func (s *stream) releaseChunk() { <-s.slots; <-s.budget }

func (s *stream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(s.current.data) == 0 {
		select {
		case s.current = <-s.queue:
		default:
			select {
			case s.current = <-s.queue:
			case <-s.done:
				// Close preserves all bytes accepted before the terminal cause.
				select {
				case s.current = <-s.queue:
				default:
					s.mu.Lock()
					err := s.err
					s.mu.Unlock()
					if err == nil {
						err = io.EOF
					}
					return 0, err
				}
			}
		}
	}
	s.at = s.current.at
	n := copy(p, s.current.data)
	s.current.data = s.current.data[n:]
	if len(s.current.data) == 0 {
		s.current = chunk{}
		s.releaseChunk()
	}
	return n, nil
}

// Called only by the consumer, including on write errors and panics.
func (s *stream) dispose() {
	s.finish(io.ErrClosedPipe)
	s.stopCancel()
	if len(s.current.data) > 0 {
		s.current = chunk{}
		s.releaseChunk()
	}
	for {
		select {
		case <-s.queue:
			s.releaseChunk()
		default:
			return
		}
	}
}
