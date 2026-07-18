package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
)

type RawRecord struct {
	SourcePluginID   string
	SourceInstanceID string
	OriginalID       string
	Domain           string
	ObservedUTC      time.Time
	Payload          json.RawMessage
}

type Source interface {
	ID() string
	Type() string
	Start(context.Context, chan<- RawRecord) error
}

type Domain interface {
	ID() string
	Domain() string
	Normalize(context.Context, RawRecord) ([]events.Envelope, error)
}

// Context combines already canonical messages without changing their source
// domain. Derived messages must carry model, provenance, validity, and quality.
type Context interface {
	ID() string
	Process(context.Context, events.Envelope) ([]events.Envelope, error)
}

type Registry struct {
	mu       sync.RWMutex
	sources  map[string]Source
	domains  map[string]Domain
	contexts map[string]Context
}

func NewRegistry() *Registry {
	return &Registry{sources: make(map[string]Source), domains: make(map[string]Domain), contexts: make(map[string]Context)}
}

func (r *Registry) RegisterContext(contextPlugin Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.contexts[contextPlugin.ID()]; exists {
		return fmt.Errorf("context %q already registered", contextPlugin.ID())
	}
	r.contexts[contextPlugin.ID()] = contextPlugin
	return nil
}

func (r *Registry) RegisterSource(source Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sources[source.ID()]; exists {
		return fmt.Errorf("source %q already registered", source.ID())
	}
	r.sources[source.ID()] = source
	return nil
}

func (r *Registry) RegisterDomain(domain Domain) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.domains[domain.Domain()]; exists {
		return fmt.Errorf("domain %q already registered", domain.Domain())
	}
	r.domains[domain.Domain()] = domain
	return nil
}

func (r *Registry) Sources() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Source, 0, len(r.sources))
	for _, source := range r.sources {
		result = append(result, source)
	}
	return result
}

func (r *Registry) Domain(id string) (Domain, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	domain, ok := r.domains[id]
	return domain, ok
}

func (r *Registry) Contexts() []Context {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Context, 0, len(r.contexts))
	for _, contextPlugin := range r.contexts {
		result = append(result, contextPlugin)
	}
	return result
}
