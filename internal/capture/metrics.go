package capture

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

const (
	maxCaptureHeaders  = 64 << 10
	maxCaptureJSON     = 4 << 20
	maxCaptureSSEEvent = 1 << 20
)

// captureMetrics observes a reconstructed HTTP stream without changing it.
// Write consumes bytes; finish returns metadata for the completed log event.
// Parsing is best-effort: invalid or oversized payloads never interrupt capture.
// ponytail: bounded JSON/SSE buffers; larger payloads omit metrics.
type captureMetrics struct {
	metadata                         Metadata
	response                         bool
	now                              func() time.Time
	headersDone, streaming, disabled bool
	buffer, data                     []byte
	firstToken, finishedAt           time.Time
}

// newCaptureMetrics accepts a clock so tests can drive real stream ingestion
// with deterministic arrival times. A nil clock uses wall time in production.
func newCaptureMetrics(metadata Metadata, streamType string, now func() time.Time) *captureMetrics {
	if now == nil {
		now = time.Now
	}
	return &captureMetrics{metadata: metadata, response: streamType == "response", now: now}
}

func (c *captureMetrics) Write(p []byte) (int, error) {
	n := len(p)
	if c.disabled {
		return n, nil
	}
	c.buffer = append(c.buffer, p...)
	if !c.headersDone && !c.readHeaders() {
		return n, nil
	}
	if !c.streaming {
		if len(c.buffer) > maxCaptureJSON {
			c.disable()
		}
		return n, nil
	}
	c.readSSEEvents()
	return n, nil
}

func (c *captureMetrics) readHeaders() bool {
	header, body, ok := bytes.Cut(c.buffer, []byte("\r\n\r\n"))
	if !ok {
		if len(c.buffer) > maxCaptureHeaders {
			c.disable()
		}
		return false
	}
	if len(header) > maxCaptureHeaders {
		c.disable()
		return false
	}
	for _, line := range strings.Split(string(header), "\r\n") {
		name, value, _ := strings.Cut(line, ":")
		if c.response && strings.EqualFold(name, "Content-Type") {
			c.streaming = strings.EqualFold(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]), "text/event-stream")
		}
	}
	c.buffer = bytes.Clone(body)
	c.headersDone = true
	return true
}

func (c *captureMetrics) readSSEEvents() {
	for {
		line, rest, ok := bytes.Cut(c.buffer, []byte{'\n'})
		if !ok {
			break
		}
		c.buffer = rest
		c.readSSELine(bytes.TrimSuffix(line, []byte{'\r'}))
		if c.disabled {
			return
		}
	}
	if len(c.buffer) > maxCaptureSSEEvent {
		c.disable()
	}
}

// readSSELine assembles data fields until the blank line that ends an event.
func (c *captureMetrics) readSSELine(line []byte) {
	if len(line) > maxCaptureSSEEvent {
		c.disable()
		return
	}
	if len(line) == 0 {
		c.parse(c.data, c.now())
		c.data = c.data[:0]
	} else if value, ok := bytes.CutPrefix(line, []byte("data:")); ok {
		value = bytes.TrimPrefix(value, []byte{' '})
		if len(c.data)+len(value) > maxCaptureSSEEvent {
			c.disable()
			return
		}
		c.data = append(c.data, value...)
		c.data = append(c.data, '\n')
	}
}

func (c *captureMetrics) disable() {
	c.disabled = true
	c.buffer, c.data = nil, nil
}

func (c *captureMetrics) finish(complete bool) Metadata {
	if !complete || c.disabled {
		return c.metadata
	}
	if c.headersDone && !c.streaming {
		c.parse(c.buffer, c.now())
	}
	if speed := c.generationSpeed(); speed != nil {
		c.metadata.TokensPerSecond = speed
	}
	return c.metadata
}

func (c *captureMetrics) generationSpeed() *float64 {
	if !c.streaming || c.firstToken.IsZero() || !c.finishedAt.After(c.firstToken) || c.metadata.OutputTokens == nil {
		return nil
	}
	speed := float64(*c.metadata.OutputTokens) / c.finishedAt.Sub(c.firstToken).Seconds()
	return &speed
}

func (c *captureMetrics) parse(data []byte, now time.Time) {
	var p inferencePayload
	if json.Unmarshal(data, &p) != nil {
		return
	}
	if !c.response {
		c.metadata.Model = p.Model
		return
	}
	p.applyTo(&c.metadata)
	if !c.streaming {
		return
	}
	hasContent, finished := p.applyStreamTo(&c.metadata)
	if hasContent && !now.IsZero() && c.firstToken.IsZero() && !c.metadata.RequestStartedAt.IsZero() {
		c.firstToken = now
		ttft := float64(now.Sub(c.metadata.RequestStartedAt)) / float64(time.Millisecond)
		if ttft >= 0 {
			c.metadata.TimeToFirstTokenMS = &ttft
		}
	}
	if c.finishedAt.IsZero() && finished {
		c.finishedAt = now
	}
}
