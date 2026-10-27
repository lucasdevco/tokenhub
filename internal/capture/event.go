package capture

import "time"

// Metadata contains information about a request for logging
type Metadata struct {
	ID                       string     `json:"id"`
	Provider                 string     `json:"provider,omitempty"`
	UserAgent                string     `json:"user_agent,omitempty"`
	Method                   string     `json:"method"`
	SourceURL                string     `json:"source_url"`
	DestinationURL           string     `json:"target_url"`
	RequestStartedAt         time.Time  `json:"request_started_at"`
	UpstreamResponseAt       *time.Time `json:"upstream_response_at,omitempty"`
	UpstreamHeaderDurationMS int64      `json:"upstream_header_duration_ms,omitempty"`
	ResponseStatus           string     `json:"response_status,omitempty"`
	ResponseStatusCode       int        `json:"response_status_code,omitempty"`
	RequestContentEncoding   string     `json:"request_content_encoding,omitempty"`
	ResponseContentEncoding  string     `json:"response_content_encoding,omitempty"`
	Model                    string     `json:"model,omitempty"`
	InputTokens              *int64     `json:"input_tokens,omitempty"`
	OutputTokens             *int64     `json:"output_tokens,omitempty"`
	RoutingOverheadMS        *float64   `json:"routing_overhead_ms,omitempty"`
	TimeToFirstTokenMS       *float64   `json:"time_to_first_token_ms,omitempty"`
	TokensPerSecond          *float64   `json:"tokens_per_second,omitempty"`
	FinishReason             string     `json:"finish_reason,omitempty"`
}

// Event is a small lifecycle record. Bodies live exclusively in the .bin files.
type Event struct {
	Event        string     `json:"event"`
	StreamType   string     `json:"stream_type"`
	Metadata     Metadata   `json:"metadata"`
	Timestamp    time.Time  `json:"timestamp"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	DurationMS   int64      `json:"duration_ms,omitempty"`
	BytesWritten int64      `json:"bytes_written"`
	Completed    bool       `json:"completed"`
	Error        string     `json:"error,omitempty"`
	CaptureError string     `json:"capture_error,omitempty"`
	WaitMS       float64    `json:"capture_wait_ms,omitempty"`
	Filename     string     `json:"filename"`
}
