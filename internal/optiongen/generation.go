package optiongen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type generationPrompt struct {
	system, user string
	maxTokens    int
	temperature  float64
}

type generationOutput struct {
	RequestFingerprint string    `json:"request_fingerprint"`
	Model              string    `json:"model"`
	Endpoint           string    `json:"endpoint"`
	RequestedAt        time.Time `json:"requested_at"`
	DurationMS         int64     `json:"duration_ms"`
	PromptTokens       int       `json:"prompt_tokens,omitempty"`
	CompletionTokens   int       `json:"completion_tokens,omitempty"`
	TotalTokens        int       `json:"total_tokens,omitempty"`
	RawResponse        string    `json:"raw_response"`
}

func (o generationOutput) fingerprint() string { return o.RequestFingerprint }

// generateRecord reuses a saved response only when the exact request matches.
// The bool reports whether this invocation sent a request.
func generateRecord[T interface{ fingerprint() string }](ctx context.Context, c Client, path string, resume bool, prompt generationPrompt, decode func(generationOutput) (T, error)) (T, bool, error) {
	var zero T
	endpoint, err := c.chatEndpoint()
	if err != nil {
		return zero, false, err
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		return zero, false, errors.New("WRE_MODEL (model) is required")
	}
	if c.MaxTokens > 0 {
		prompt.maxTokens = c.MaxTokens
	}
	temperature := c.Temperature
	if temperature <= 0 {
		temperature = prompt.temperature
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": prompt.system},
			{"role": "user", "content": prompt.user},
		},
		"temperature":     temperature,
		"max_tokens":      prompt.maxTokens,
		"response_format": map[string]string{"type": "json_object"},
	}
	if effort := strings.TrimSpace(c.ReasoningEffort); effort != "" {
		body["reasoning_effort"] = effort
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return zero, false, err
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(append([]byte(endpoint+"\n"), raw...)))
	if resume {
		data, err := os.ReadFile(path)
		if err == nil {
			var saved T
			if err := json.Unmarshal(data, &saved); err != nil {
				return zero, false, fmt.Errorf("read saved response %s: %w", path, err)
			}
			if saved.fingerprint() != fingerprint {
				return zero, false, fmt.Errorf("saved response %s does not match the current request; choose a new output directory or run without -resume", path)
			}
			return saved, false, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return zero, false, err
		}
	}

	started := time.Now().UTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return zero, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(c.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Minute}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, true, fmt.Errorf("request ds: %w", err)
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, true, fmt.Errorf("read ds response: %w", err)
	}
	durationMS := time.Since(started).Milliseconds()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(response)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		return zero, true, fmt.Errorf("ds http %d: %s", resp.StatusCode, snippet)
	}
	var envelope struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return zero, true, fmt.Errorf("decode ds envelope: %w", err)
	}
	if len(envelope.Choices) == 0 {
		return zero, true, errors.New("ds returned empty choices")
	}
	choice := envelope.Choices[0]
	out := generationOutput{
		RequestFingerprint: fingerprint,
		Model:              model,
		Endpoint:           endpoint,
		RequestedAt:        started,
		DurationMS:         durationMS,
		PromptTokens:       envelope.Usage.PromptTokens,
		CompletionTokens:   envelope.Usage.CompletionTokens,
		TotalTokens:        envelope.Usage.TotalTokens,
		RawResponse:        choice.Message.Content,
	}
	record, err := decode(out)
	if err != nil {
		content := out.RawResponse
		if len(content) > 240 {
			content = "..." + content[len(content)-240:]
		}
		return zero, true, fmt.Errorf("parse ds response (finish_reason=%s len=%d tokens=%d/%d): %w | tail=%q",
			choice.FinishReason, len(out.RawResponse), out.PromptTokens, out.CompletionTokens, err, content)
	}
	if err := writeJSONFile(path, record); err != nil {
		return zero, true, err
	}
	return record, true, nil
}
