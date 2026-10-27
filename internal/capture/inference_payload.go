package capture

import "encoding/json"

type inferenceUsage struct {
	Input         *int64 `json:"input_tokens"`
	Output        *int64 `json:"output_tokens"`
	Prompt        *int64 `json:"prompt_tokens"`
	Completion    *int64 `json:"completion_tokens"`
	CacheRead     *int64 `json:"cache_read_input_tokens"`
	CacheCreation *int64 `json:"cache_creation_input_tokens"`
}

// inferencePayload normalizes OpenAI Chat Completions, Responses, and Claude
// payloads. It only interprets protocol fields; stream framing, clocks, and
// persistence belong to the capture and logging modules.
type inferencePayload struct {
	Type              string            `json:"type"`
	Model             string            `json:"model"`
	Status            string            `json:"status"`
	StopReason        string            `json:"stop_reason"`
	Usage             *inferenceUsage   `json:"usage"`
	Delta             json.RawMessage   `json:"delta"`
	Message           *inferencePayload `json:"message"`
	Response          *inferencePayload `json:"response"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Content   string            `json:"content"`
			Reasoning string            `json:"reasoning_content"`
			ToolCalls []json.RawMessage `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

func (p inferencePayload) applyTo(metadata *Metadata) {
	if p.Message != nil {
		p.Message.applyTo(metadata)
	}
	if p.Response != nil {
		p.Response.applyTo(metadata)
	}
	if p.Model != "" {
		metadata.Model = p.Model
	}
	if p.Usage != nil {
		p.Usage.applyTo(metadata)
	}
	if reason := p.finishReason(); reason != "" {
		metadata.FinishReason = reason
	}
}

func (u inferenceUsage) applyTo(metadata *Metadata) {
	input, output := u.Input, u.Output
	if input == nil {
		input = u.Prompt
	}
	if output == nil {
		output = u.Completion
	}
	if input != nil && *input >= 0 {
		total := *input
		for _, cache := range []*int64{u.CacheRead, u.CacheCreation} {
			if cache != nil && *cache >= 0 {
				total += *cache
			}
		}
		metadata.InputTokens = &total
	}
	if output != nil && *output >= 0 {
		metadata.OutputTokens = output
	}
}

// finishReason preserves the priority: Responses status, Chat choice, Claude stop.
func (p inferencePayload) finishReason() string {
	switch p.Status {
	case "completed", "incomplete", "failed":
		if p.IncompleteDetails != nil && p.IncompleteDetails.Reason != "" {
			return p.IncompleteDetails.Reason
		}
		return p.Status
	}
	for _, choice := range p.Choices {
		if choice.FinishReason != "" {
			return choice.FinishReason
		}
	}
	return p.StopReason
}

// applyStreamTo normalizes stream deltas and returns content/terminal signals.
func (p inferencePayload) applyStreamTo(metadata *Metadata) (hasContent, finished bool) {
	var delta struct {
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	}
	_ = json.Unmarshal(p.Delta, &delta)
	if delta.StopReason != "" {
		metadata.FinishReason = delta.StopReason
	}
	content := delta.Text != "" || delta.Thinking != "" || delta.PartialJSON != ""
	switch p.Type {
	case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		var text string
		if json.Unmarshal(p.Delta, &text) == nil {
			content = text != ""
		}
	}
	for _, choice := range p.Choices {
		content = content || choice.Delta.Content != "" || choice.Delta.Reasoning != "" || len(choice.Delta.ToolCalls) > 0
	}
	return content, metadata.FinishReason != "" || p.Type == "message_stop"
}
