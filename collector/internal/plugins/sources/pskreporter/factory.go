package pskreporter

import (
	"fmt"
	"log/slog"

	"github.com/ion-command/ion-command/collector/internal/config"
)

// NewMQTT wires the injected decoder and the public MQTT transport into the
// prepared adapter boundary.
func NewMQTT(sourceConfig config.Source, logger *slog.Logger) (*Adapter, error) {
	if sourceConfig.Type != "pskreporter.mqtt" {
		return nil, fmt.Errorf("unsupported PSKReporter source type %q", sourceConfig.Type)
	}
	return &Adapter{
		InstanceID: sourceConfig.ID,
		Decoder:    NewSpotDecoder(),
		Transport: &MQTTTransport{
			Broker:   sourceConfig.Broker,
			Topic:    sourceConfig.Topic,
			ClientID: sourceConfig.ClientID,
			Logger:   logger,
		},
	}, nil
}
