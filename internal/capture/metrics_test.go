package capture

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestCaptureMetricsJSON(t *testing.T) {
	for _, tc := range []struct {
		name, body, model, reason string
		input, output             int64
	}{
		{"chat", `{"model":"deepseek-chat","usage":{"prompt_tokens":123,"completion_tokens":45},"choices":[{"finish_reason":"stop"}]}`, "deepseek-chat", "stop", 123, 45},
		{"claude", `{"model":"claude","usage":{"input_tokens":10,"cache_read_input_tokens":20,"cache_creation_input_tokens":30,"output_tokens":40},"stop_reason":"end_turn"}`, "claude", "end_turn", 60, 40},
		{"responses", `{"model":"gpt","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":0,"output_tokens":9}}`, "gpt", "max_output_tokens", 0, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCaptureMetrics(Metadata{}, "response", nil)
			raw := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n" + tc.body
			for i := range raw {
				_, _ = c.Write([]byte{raw[i]})
			}
			m := c.finish(true)
			if m.Model != tc.model || m.FinishReason != tc.reason || m.InputTokens == nil || *m.InputTokens != tc.input || m.OutputTokens == nil || *m.OutputTokens != tc.output {
				t.Fatalf("unexpected metrics: %+v", m)
			}
			if m.TimeToFirstTokenMS != nil || m.TokensPerSecond != nil {
				t.Fatal("JSON responses must not claim streaming timings")
			}
		})
	}
}

func TestCaptureMetricsStreamingTimings(t *testing.T) {
	start := time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, initial, token, final string
		input, output               int64
		reason                      string
	}{
		{"chat", `{"choices":[{"delta":{"role":"assistant"}}]}`, `{"model":"deepseek-chat","choices":[{"delta":{"reasoning_content":"think"}}]}`, `{"usage":{"prompt_tokens":10,"completion_tokens":20},"choices":[{"finish_reason":"stop"}]}`, 10, 20, "stop"},
		{"claude", `{"type":"message_start","message":{"model":"claude","usage":{"input_tokens":10,"cache_read_input_tokens":30,"output_tokens":1}}}`, `{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{"}}`, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}`, 40, 20, "tool_use"},
		{"responses", `{"type":"response.created","response":{"model":"gpt","status":"in_progress"}}`, `{"type":"response.output_text.delta","delta":"hello"}`, `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":10,"output_tokens":20}}}`, 10, 20, "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := start
			c := newCaptureMetrics(Metadata{RequestStartedAt: start}, "response", func() time.Time { return now })
			_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
			now = start.Add(time.Second)
			_, _ = io.WriteString(c, "data: "+tc.initial+"\n\n")
			now = start.Add(2 * time.Second)
			// Split an event across writes: only the event terminator records arrival.
			_, _ = io.WriteString(c, "data: "+tc.token)
			_, _ = io.WriteString(c, "\n\n")
			now = start.Add(4 * time.Second)
			_, _ = io.WriteString(c, "data: "+tc.final+"\n\n")
			// Completion and keep-alive delay must not inflate generation time.
			now = start.Add(10 * time.Second)
			_, _ = io.WriteString(c, ": keep-alive\n\n")
			interrupted := c.finish(false)
			if interrupted.TokensPerSecond != nil {
				t.Fatal("interrupted capture must not report speed")
			}
			m := c.finish(true)
			if m.TimeToFirstTokenMS == nil || *m.TimeToFirstTokenMS != 2000 || m.TokensPerSecond == nil || *m.TokensPerSecond != 10 || m.InputTokens == nil || *m.InputTokens != tc.input || m.OutputTokens == nil || *m.OutputTokens != tc.output || m.FinishReason != tc.reason {
				t.Fatalf("unexpected metrics: %+v", m)
			}
		})
	}
}

func TestCaptureMetricsStreamingMissingOrIncompleteData(t *testing.T) {
	start := time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)
	content := `{"choices":[{"delta":{"content":"hi"}}]}`
	withUsage := `{"usage":{"completion_tokens":20},"choices":[{"finish_reason":"stop"}]}`
	withoutUsage := `{"choices":[{"finish_reason":"stop"}]}`
	for _, tc := range []struct {
		name, token, final string
		elapsed            time.Duration
		complete, wantTTFT bool
	}{
		{"no content", `{"choices":[{"delta":{"role":"assistant"}}]}`, withUsage, time.Second, true, false},
		{"no usage", content, withoutUsage, time.Second, true, true},
		{"no terminal event", content, `{"usage":{"completion_tokens":20}}`, time.Second, true, true},
		{"zero duration", content, withUsage, 0, true, true},
		{"interrupted capture", content, withUsage, time.Second, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := start.Add(time.Second)
			c := newCaptureMetrics(Metadata{RequestStartedAt: start}, "response", func() time.Time { return now })
			_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: "+tc.token+"\n\n")
			now = now.Add(tc.elapsed)
			_, _ = io.WriteString(c, "data: "+tc.final+"\n\n")
			metrics := c.finish(tc.complete)
			if (metrics.TimeToFirstTokenMS != nil) != tc.wantTTFT || metrics.TokensPerSecond != nil {
				t.Fatalf("missing/incomplete data produced invalid timings: %+v", metrics)
			}
		})
	}
}

func TestCaptureMetricsSSEFramingAndLimits(t *testing.T) {
	c := newCaptureMetrics(Metadata{RequestStartedAt: time.Now().Add(-time.Second)}, "response", nil)
	raw := "HTTP/1.1 200 OK\r\ncontent-type:\ttext/event-stream; charset=utf-8\r\n\r\n: ping\r\n\r\ndata: {\"model\":\"test\",\r\ndata: \"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\r\n\r\ndata: [DONE]\n\n"
	for i := range raw {
		_, _ = c.Write([]byte{raw[i]})
	}
	metrics := c.finish(true)
	if metrics.Model != "test" || metrics.TimeToFirstTokenMS == nil {
		t.Fatalf("split/multiline SSE not parsed: %+v", metrics)
	}
	_, _ = c.Write([]byte("data: " + strings.Repeat("x", 1<<20) + "\n\n"))
	if !c.disabled {
		t.Fatal("oversized SSE must disable parsing")
	}
	c = newCaptureMetrics(Metadata{}, "response", nil)
	_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\n\r\n" + strings.Repeat("x", 4<<20+1)))
	if !c.disabled {
		t.Fatal("oversized JSON must disable parsing")
	}
}
