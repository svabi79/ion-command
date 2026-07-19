package pskreporter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	DefaultBroker = "tcp://mqtt.pskreporter.info:1883"
	DefaultTopic  = "pskr/filter/v2/#"
)

// MQTTTransport subscribes to the public PSKReporter MQTT feed and hands every
// message payload to the adapter callback. Decode failures of individual
// frames are logged and skipped so one malformed spot cannot kill the source.
type MQTTTransport struct {
	Broker   string
	Topic    string
	ClientID string
	Logger   *slog.Logger
}

func (t *MQTTTransport) Run(ctx context.Context, handle func([]byte) error) error {
	broker := t.Broker
	if broker == "" {
		broker = DefaultBroker
	}
	topic := t.Topic
	if topic == "" {
		topic = DefaultTopic
	}
	clientID := t.ClientID
	if clientID == "" {
		clientID = fmt.Sprintf("ion-command-%d", time.Now().UnixNano())
	}
	logger := t.Logger
	if logger == nil {
		logger = slog.Default()
	}

	options := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(clientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(30 * time.Second).
		SetConnectTimeout(15 * time.Second).
		SetKeepAlive(60 * time.Second).
		SetOrderMatters(false)
	options.OnConnect = func(client mqtt.Client) {
		token := client.Subscribe(topic, 0, func(_ mqtt.Client, message mqtt.Message) {
			if err := handle(message.Payload()); err != nil {
				logger.Warn("PSKReporter frame skipped", "error", err)
			}
		})
		token.Wait()
		if token.Error() != nil {
			logger.Error("PSKReporter subscribe failed", "topic", topic, "error", token.Error())
			return
		}
		logger.Info("PSKReporter MQTT subscribed", "broker", broker, "topic", topic)
	}
	options.OnConnectionLost = func(_ mqtt.Client, err error) {
		logger.Warn("PSKReporter MQTT connection lost", "error", err)
	}

	client := mqtt.NewClient(options)
	token := client.Connect()
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("connect to PSKReporter broker %s: %w", broker, token.Error())
	}
	<-ctx.Done()
	client.Disconnect(250)
	return nil
}
