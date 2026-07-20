package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/recording"
	"github.com/ion-command/ion-command/collector/internal/stream"
	"github.com/ion-command/ion-command/collector/internal/telemetry"
)

type SourceState struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type Pipeline struct {
	registry     *plugins.Registry
	rawQueue     chan plugins.RawRecord
	workers      int
	hub          *stream.Hub
	recorder     *recording.Writer
	stats        *telemetry.Stats
	logger       *slog.Logger
	stateMu      sync.RWMutex
	retainTypes  map[string]struct{}
	sourceStates map[string]SourceState
}

func New(registry *plugins.Registry, queueCapacity, workers int, hub *stream.Hub, recorder *recording.Writer, stats *telemetry.Stats, logger *slog.Logger, retainLatest []string) *Pipeline {
	retain := make(map[string]struct{}, len(retainLatest))
	for _, semanticType := range retainLatest {
		retain[semanticType] = struct{}{}
	}
	return &Pipeline{registry: registry, rawQueue: make(chan plugins.RawRecord, queueCapacity), workers: workers, hub: hub, recorder: recorder, stats: stats, logger: logger, sourceStates: make(map[string]SourceState), retainTypes: retain}
}

func (p *Pipeline) Run(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	for i := 0; i < p.workers; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); p.runWorker(workerCtx) }()
	}
	var sources sync.WaitGroup
	for _, source := range p.registry.Sources() {
		source := source
		p.setSourceState(source.ID(), source.Type(), "initialising", "")
		sources.Add(1)
		go func() {
			defer sources.Done()
			p.setSourceState(source.ID(), source.Type(), "active", "")
			if err := source.Start(workerCtx, p.rawQueue); err != nil && workerCtx.Err() == nil {
				p.setSourceState(source.ID(), source.Type(), "failed", err.Error())
				p.logger.Error("source stopped", "source", source.ID(), "error", err)
			} else {
				p.setSourceState(source.ID(), source.Type(), "stopped", "")
			}
		}()
	}
	<-ctx.Done()
	cancel()
	sources.Wait()
	workers.Wait()
	return p.recorder.Close()
}

func (p *Pipeline) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case raw := <-p.rawQueue:
			p.stats.IncRaw()
			p.stats.SetQueueDepth(len(p.rawQueue))
			domain, ok := p.registry.Domain(raw.Domain)
			if !ok {
				p.stats.IncInvalid()
				p.logger.Warn("no domain plugin", "domain", raw.Domain)
				continue
			}
			messages, err := domain.Normalize(ctx, raw)
			if err != nil {
				p.stats.IncInvalid()
				p.logger.Warn("normalization failed", "domain", raw.Domain, "error", err)
				continue
			}
			for _, message := range messages {
				p.publish(message)
				for _, contextPlugin := range p.registry.Contexts() {
					derived, err := contextPlugin.Process(ctx, message)
					if err != nil {
						p.logger.Warn("context processing failed", "context", contextPlugin.ID(), "error", err)
						continue
					}
					for _, derivedMessage := range derived {
						p.publish(derivedMessage)
					}
				}
			}
		}
	}
}

func (p *Pipeline) publish(message events.Envelope) {
	if err := message.Validate(); err != nil {
		p.stats.IncInvalid()
		p.logger.Warn("canonical event rejected", "messageId", message.MessageID, "error", err)
		return
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		p.stats.IncInvalid()
		return
	}
	if err := p.recorder.Write(message.Time.ObservedUTC, encoded); err != nil {
		p.logger.Error("recording failed", "error", err)
	} else if p.recorder.Enabled() {
		p.stats.IncRecorded()
	}
	retainKey := ""
	var retainExpiry time.Time
	if _, retainable := p.retainTypes[message.SemanticType]; retainable {
		retainKey = message.SemanticType + "|" + message.EntityID
		if message.Time.ValidUntilUTC != nil {
			retainExpiry = *message.Time.ValidUntilUTC
		}
	}
	p.hub.Publish(encoded, retainKey, retainExpiry)
	p.stats.IncPublished()
}

func (p *Pipeline) Sources() []SourceState {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	result := make([]SourceState, 0, len(p.sourceStates))
	for _, state := range p.sourceStates {
		result = append(result, state)
	}
	return result
}

func (p *Pipeline) setSourceState(id, sourceType, state, message string) {
	p.stateMu.Lock()
	p.sourceStates[id] = SourceState{ID: id, Type: sourceType, State: state, Message: message}
	p.stateMu.Unlock()
}

func (p *Pipeline) Verify() error {
	if len(p.registry.Sources()) == 0 {
		return fmt.Errorf("no source plugins registered")
	}
	return nil
}
