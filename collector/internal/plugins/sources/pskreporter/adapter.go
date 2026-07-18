// Package pskreporter is the integration boundary for the subscribed live
// feed. The wire decoder is deliberately injected because the canonical core
// must not depend on a feed-specific framing or subscription contract.
package pskreporter

import (
	"context"
	"fmt"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Decoder interface {
	Decode(frame []byte, sourceInstanceID string) ([]plugins.RawRecord, error)
}

type Transport interface {
	Run(context.Context, func([]byte) error) error
}

type Adapter struct {
	InstanceID string
	Decoder    Decoder
	Transport  Transport
}

func (a *Adapter) ID() string   { return a.InstanceID }
func (a *Adapter) Type() string { return "pskreporter.live" }

func (a *Adapter) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	if a.Decoder == nil || a.Transport == nil {
		return fmt.Errorf("PSKReporter adapter requires an injected decoder and transport")
	}
	return a.Transport.Run(ctx, func(frame []byte) error {
		records, err := a.Decoder.Decode(frame, a.InstanceID)
		if err != nil {
			return err
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
}
