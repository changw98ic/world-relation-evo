// Package modelreply defines the structured content a model may add to an action.
package modelreply

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type Content struct {
	Text         string         `json:"text"`
	Statement    *sim.Statement `json:"statement,omitempty"`
	BasisEventID int64          `json:"basis_event_id,omitempty"`
}

func Decode(text string) (Content, error) {
	contentText := strings.TrimSpace(text)
	if strings.HasPrefix(contentText, "```") {
		if idx := strings.Index(contentText, "\n"); idx >= 0 {
			contentText = contentText[idx+1:]
		}
		contentText = strings.TrimSuffix(strings.TrimSpace(contentText), "```")
		contentText = strings.TrimSpace(contentText)
	}
	var a *Content
	decoder := json.NewDecoder(strings.NewReader(contentText))
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&a); e != nil {
		return Content{}, fmt.Errorf("invalid content JSON: %w", e)
	}
	var trailing json.RawMessage
	if e := decoder.Decode(&trailing); e != io.EOF {
		if e != nil {
			return Content{}, fmt.Errorf("invalid trailing content JSON: %w", e)
		}
		return Content{}, fmt.Errorf("content response must contain exactly one JSON object")
	}
	if a == nil {
		return Content{}, fmt.Errorf("content response must be a JSON object")
	}
	return *a, nil
}
