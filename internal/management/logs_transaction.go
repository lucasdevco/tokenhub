package management

import (
	"sort"
	"time"

	"github.com/lucasdevco/tokenhub/internal/capture"
)

type Event = capture.Event

type Transaction struct {
	Key           string           `json:"key"`
	Metadata      capture.Metadata `json:"metadata"`
	Started       time.Time        `json:"started"`
	State         string           `json:"state"`
	CaptureState  string           `json:"capture_state"`
	CaptureWaitMS float64          `json:"capture_wait_ms"`
	RequestFile   string           `json:"request_file,omitempty"`
	ResponseFile  string           `json:"response_file,omitempty"`
	RequestBytes  int64            `json:"request_bytes"`
	ResponseBytes int64            `json:"response_bytes"`
	DurationMS    int64            `json:"duration_ms"`
	Events        []Event          `json:"events,omitempty"`
	MetadataFiles []string         `json:"metadata_files,omitempty"`
	Problems      []string         `json:"problems,omitempty"`
}

// summarize folds a newly grouped transaction's events into its display fields.
// It only uses captured events and file sizes; it never reads or writes files.
func (t *Transaction) summarize(files map[string]File) {
	sort.SliceStable(t.Events, func(i, j int) bool { return t.Events[i].Timestamp.Before(t.Events[j].Timestamp) })
	if len(t.Events) > 0 {
		first := t.Events[0]
		t.Metadata = first.Metadata
		t.Started = first.StartedAt
		if t.Started.IsZero() {
			t.Started = first.Timestamp
		}
	}
	requestDone, responseDone := false, false
	t.CaptureState = "recording"
	captureFailed := false
	captureIncomplete := false
	requestModel := ""
	for _, event := range t.Events {
		t.mergeEvent(event)
		if event.CaptureError != "" {
			captureFailed = true
			t.Problems = append(t.Problems, event.CaptureError)
		}
		if event.StreamType != "proxy" && event.Error != "" {
			captureIncomplete = true
		}
		t.CaptureWaitMS += event.WaitMS
		if model := event.Metadata.Model; event.StreamType == "request" && model != "" {
			requestModel = model
		}
		switch event.StreamType {
		case "request":
			t.RequestFile = event.Filename
			requestDone = requestDone || event.Event == "request_completed"
		case "response":
			t.ResponseFile = event.Filename
			responseDone = responseDone || event.Event == "response_completed"
		}
	}
	if t.Metadata.Model == "" {
		t.Metadata.Model = requestModel
	}

	if captureFailed {
		t.CaptureState = "failed"
	} else if requestDone && responseDone {
		if captureIncomplete {
			t.CaptureState = "incomplete"
		} else {
			t.CaptureState = "completed"
		}
	}
	t.RequestBytes = files[t.RequestFile].Size
	t.ResponseBytes = files[t.ResponseFile].Size
}

// mergeEvent updates the response snapshot, capture errors, and elapsed time.
// Request/response completion is folded separately by summarize.
func (t *Transaction) mergeEvent(event Event) {
	if !event.Metadata.RequestStartedAt.IsZero() {
		t.Started = event.Metadata.RequestStartedAt
	}
	if event.StreamType == "response" && event.Metadata.ResponseStatusCode != 0 {
		t.Metadata = event.Metadata
	}
	if event.Event == "proxy_completed" {
		if event.Completed {
			t.State = "completed"
		} else {
			t.State = "error"
		}
	}
	if event.Error != "" {
		t.Problems = append(t.Problems, event.Error)
	}
	if event.Event == "proxy_completed" && event.CompletedAt != nil {
		duration := event.CompletedAt.Sub(t.Started).Milliseconds()
		if duration > t.DurationMS {
			t.DurationMS = duration
		}
	}
}
