package telemetry

import (
	"sync/atomic"
	"time"
)

type Stats struct {
	started            time.Time
	rawReceived        atomic.Uint64
	canonicalPublished atomic.Uint64
	invalid            atomic.Uint64
	droppedRaw         atomic.Uint64
	droppedClients     atomic.Uint64
	recorded           atomic.Uint64
	connectedClients   atomic.Int64
	queueDepth         atomic.Int64
}

type Snapshot struct {
	UptimeSeconds         int64  `json:"uptimeSeconds"`
	RawReceived           uint64 `json:"rawReceived"`
	CanonicalPublished    uint64 `json:"canonicalPublished"`
	InvalidEvents         uint64 `json:"invalidEvents"`
	DroppedRaw            uint64 `json:"droppedRaw"`
	DroppedClientMessages uint64 `json:"droppedClientMessages"`
	RecordedEvents        uint64 `json:"recordedEvents"`
	ConnectedClients      int64  `json:"connectedClients"`
	RawQueueDepth         int64  `json:"rawQueueDepth"`
}

func New() *Stats                               { return &Stats{started: time.Now()} }
func (s *Stats) IncRaw()                        { s.rawReceived.Add(1) }
func (s *Stats) IncPublished()                  { s.canonicalPublished.Add(1) }
func (s *Stats) IncInvalid()                    { s.invalid.Add(1) }
func (s *Stats) IncDroppedRaw()                 { s.droppedRaw.Add(1) }
func (s *Stats) AddDroppedClients(count uint64) { s.droppedClients.Add(count) }
func (s *Stats) IncRecorded()                   { s.recorded.Add(1) }
func (s *Stats) ClientConnected()               { s.connectedClients.Add(1) }
func (s *Stats) ClientDisconnected()            { s.connectedClients.Add(-1) }
func (s *Stats) SetQueueDepth(depth int)        { s.queueDepth.Store(int64(depth)) }
func (s *Stats) Snapshot() Snapshot {
	return Snapshot{UptimeSeconds: int64(time.Since(s.started).Seconds()), RawReceived: s.rawReceived.Load(), CanonicalPublished: s.canonicalPublished.Load(), InvalidEvents: s.invalid.Load(), DroppedRaw: s.droppedRaw.Load(), DroppedClientMessages: s.droppedClients.Load(), RecordedEvents: s.recorded.Load(), ConnectedClients: s.connectedClients.Load(), RawQueueDepth: s.queueDepth.Load()}
}
