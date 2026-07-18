package hfpropagation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
)

// Context is intentionally a low-confidence bootstrap correlation. It proves
// cross-domain composition without pretending to be a propagation model.
type Context struct {
	mu              sync.RWMutex
	kp              float64
	sourceMessageID string
	observed        time.Time
	hasState        bool
}

func New() *Context           { return &Context{} }
func (c *Context) ID() string { return "context.hf-propagation.bootstrap" }

func (c *Context) Process(_ context.Context, message events.Envelope) ([]events.Envelope, error) {
	if message.SemanticType == "spaceweather.state" {
		kp, ok := numeric(message.Properties["kp"])
		if !ok {
			return nil, fmt.Errorf("spaceweather.state has no numeric kp")
		}
		c.mu.Lock()
		c.kp, c.sourceMessageID, c.observed, c.hasState = kp, message.MessageID, message.Time.ObservedUTC, true
		c.mu.Unlock()
		return nil, nil
	}
	if message.SemanticType != "radio.reception" {
		return nil, nil
	}
	c.mu.RLock()
	kp, sourceID, observed, available := c.kp, c.sourceMessageID, c.observed, c.hasState
	c.mu.RUnlock()
	if !available {
		return nil, nil
	}
	source := events.SourceRef{PluginID: c.ID(), InstanceID: "primary", OriginalID: message.MessageID}
	annotation := events.NewEnvelope(message.MessageID+":hf-context", "context", "radio.propagation.context", events.MessageAnnotation, source, message.Time.ObservedUTC)
	annotation.TargetID = message.MessageID
	validUntil := observed.Add(15 * time.Minute)
	annotation.Time.ValidUntilUTC = &validUntil
	confidence := 0.10
	measured := false
	annotation.Quality.Confidence = &confidence
	annotation.Quality.Measured = &measured
	annotation.Quality.Classification = "derived"
	annotation.Properties = map[string]any{
		"kp":               kp,
		"modelName":        "bootstrap-co-temporal-join",
		"modelVersion":     "0.1.0",
		"sourceMessageIds": []string{message.MessageID, sourceID},
		"calculatedUtc":    time.Now().UTC(),
		"warning":          "Context only; not an HF propagation prediction",
	}
	return []events.Envelope{annotation}, nil
}

func numeric(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}
