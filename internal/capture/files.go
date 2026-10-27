package capture

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lucasdevco/tokenhub/internal/database/queries"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (tx *Transaction) prefix() string {
	return tx.metadata.RequestStartedAt.Format("2006-01-02_15-04-05.000") + "_" + tx.metadata.ID
}

func (tx *Transaction) appendEvent(event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	path := filepath.Join(tx.recorder.dir, tx.prefix()+"_metadata.jsonl")
	tx.metaMu.Lock()
	defer tx.metaMu.Unlock()
	// Persist intent before touching the file. A crash leaves a recoverable job,
	// and a viewer cannot acknowledge a newer write using an older job ID.
	ticket, indexErr := tx.recorder.index.Q.BeginCaptureWrite(context.Background(), queries.BeginCaptureWriteParams{Source: tx.recorder.dir, Name: filepath.Base(path)})
	if indexErr != nil {
		log.Printf("[capture] index intent: %v", indexErr)
	}
	defer func() {
		if indexErr == nil {
			if err := tx.recorder.index.Q.EndCaptureWrite(context.Background(), ticket); err != nil {
				log.Printf("[capture] index completion: %v", err)
			}
		}
	}()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (tx *Transaction) consume(input *stream, metadata Metadata, started time.Time, firstLine string, headers http.Header, direction, encoding string) {
	event := Event{Event: direction + "_started", StreamType: direction, Metadata: metadata, Timestamp: started, StartedAt: started, Filename: tx.prefix() + "_" + direction + ".bin"}
	var file *os.File
	var output *bufio.Writer
	var metrics *captureMetrics
	defer func() {
		if recovered := recover(); recovered != nil {
			event.CaptureError = fmt.Sprintf("capture panic: %v", recovered)
		}
		tx.completeCapture(event, input, output, file, metrics)
	}()
	if err := os.MkdirAll(tx.recorder.dir, 0700); err != nil {
		event.CaptureError = err.Error()
		return
	}
	if err := tx.appendEvent(event); err != nil {
		event.CaptureError = err.Error()
		return
	}
	var err error
	file, err = os.OpenFile(filepath.Join(tx.recorder.dir, event.Filename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		event.CaptureError = err.Error()
		return
	}
	output = bufio.NewWriterSize(file, chunkSize)
	var source io.Reader = input
	opaque := false
	if encoding != "" {
		decoded, err := decompressReader(input, encoding)
		if errors.Is(err, errUnsupportedEncoding) {
			// Preserve the original bytes and headers when we cannot decode them.
			encoding, opaque = "", true
		} else if err != nil {
			event.CaptureError = err.Error()
			return
		} else {
			defer decoded.Close()
			source = decoded
		}
	}
	var prefix bytes.Buffer
	fmt.Fprintln(&prefix, firstLine+"\r")
	for name, values := range headers {
		if strings.EqualFold(name, "Proxy-Authorization") || strings.EqualFold(name, "Proxy-Authenticate") || (encoding != "" && (strings.EqualFold(name, "Content-Encoding") || strings.EqualFold(name, "Content-Length"))) {
			continue
		}
		for _, value := range values {
			fmt.Fprintf(&prefix, "%s: %s\r\n", name, value)
		}
	}
	prefix.WriteString("\r\n")
	clock := func() time.Time { return input.at }
	// Compressed decoders may read ahead across events; omit timing rather than
	// attributing them to an unrelated compressed block. Usage still parses.
	if encoding != "" {
		clock = func() time.Time { return time.Time{} }
	}
	metrics = newCaptureMetrics(metadata, direction, clock)
	if opaque {
		metrics.disable()
	}
	if _, err = io.Copy(io.MultiWriter(metrics, output), &prefix); err != nil {
		event.CaptureError = err.Error()
		return
	}
	if err := copyCaptureBody(input, source, output, metrics); err != nil {
		event.CaptureError = err.Error()
	}
}

// completeCapture flushes the file before publishing its size and final outcome.
func (tx *Transaction) completeCapture(event Event, input *stream, output *bufio.Writer, file *os.File, metrics *captureMetrics) {
	if output != nil {
		if err := output.Flush(); err != nil {
			event.CaptureError = err.Error()
		}
	}
	if file != nil {
		if info, err := file.Stat(); err == nil {
			event.BytesWritten = info.Size()
		}
		if err := file.Close(); err != nil {
			event.CaptureError = err.Error()
		}
	}
	input.mu.Lock()
	cause := input.err
	input.mu.Unlock()
	if cause != nil {
		event.Error = cause.Error()
	}
	event.Completed = event.Error == "" && event.CaptureError == ""
	if metrics != nil {
		event.Metadata = metrics.finish(event.Completed)
	}
	now := time.Now()
	event.Event, event.Timestamp, event.CompletedAt = event.StreamType+"_completed", now, &now
	event.DurationMS = now.Sub(event.StartedAt).Milliseconds()
	event.WaitMS = float64(input.waitNS.Load()) / float64(time.Millisecond)
	if err := tx.appendEvent(event); err != nil {
		log.Printf("[capture] %s metadata: %v", event.Metadata.ID, err)
	}
	if event.CaptureError != "" {
		log.Printf("[capture] %s %s: %s", event.Metadata.ID, event.StreamType, event.CaptureError)
	}
	if tx.recorder.console {
		log.Printf("[capture] %s %s: %d bytes, wait %.2f ms", event.Metadata.ID, event.StreamType, event.BytesWritten, event.WaitMS)
	}
}

// copyCaptureBody keeps metrics on arrival time and flushes when capture catches up.
func copyCaptureBody(input *stream, source io.Reader, output *bufio.Writer, metrics *captureMetrics) error {
	buffer := make([]byte, chunkSize)
	for {
		n, readErr := source.Read(buffer)
		if n > 0 {
			_, _ = metrics.Write(buffer[:n])
			if _, err := output.Write(buffer[:n]); err != nil {
				return err
			}
			// Publish partial bodies for live inspection without waiting for EOF.
			if len(input.queue) == 0 {
				if err := output.Flush(); err != nil {
					return err
				}
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				input.mu.Lock()
				cause := input.err
				input.mu.Unlock()
				if cause == nil {
					return readErr
				}
			}
			return nil
		}
	}
}
