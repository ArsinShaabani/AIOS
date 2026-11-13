// Package mqtt adapters connect AIOS to MQTT brokers for IoT devices, robots and
// smart-home gateways (Zigbee2MQTT, Tasmota, Shelly, Sonoff, ESP32, ROS2 bridges).
package mqtt

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Client is a reconnecting MQTT client with helpers for JSON payloads.
type Client struct {
	mu     sync.RWMutex
	client paho.Client
	cfg    config.MQTTConfig
	log    *slog.Logger
	subs   map[string]func(topic string, payload []byte)
}

// Connect dials the broker and waits for the connection.
func Connect(ctx context.Context, cfg config.MQTTConfig, log *slog.Logger) (*Client, error) {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Broker == "" {
		return nil, platform.Invalid("mqtt.broker", "mqtt.broker is required")
	}
	clientID := cfg.ClientID
	if clientID == "" {
		clientID = "aios-" + platform.MustRandomToken(6)
	}
	opts := paho.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetKeepAlive(time.Duration(max(cfg.KeepAlive, 15)) * time.Second).
		SetPingTimeout(10 * time.Second).
		SetOrderMatters(false)
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}
	c := &Client{cfg: cfg, log: log, subs: map[string]func(string, []byte){}}
	opts.SetOnConnectHandler(func(pc paho.Client) {
		c.log.Info("mqtt connected", "broker", cfg.Broker)
		c.resubscribe()
	})
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		c.log.Warn("mqtt connection lost", "error", err)
	})
	c.client = paho.NewClient(opts)

	token := c.client.Connect()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, platform.Unavailable("mqtt.timeout", "timed out connecting to the MQTT broker")
	case ok := <-waitToken(token):
		if !ok {
			return nil, platform.Wrap(platform.KindUnavailable, "mqtt.connect", "cannot connect to the MQTT broker", token.Error())
		}
	}
	return c, nil
}

func waitToken(t paho.Token) <-chan bool {
	ch := make(chan bool, 1)
	go func() {
		t.Wait()
		ch <- t.Error() == nil
	}()
	return ch
}

func (c *Client) resubscribe() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for topic, handler := range c.subs {
		h := handler
		c.client.Subscribe(topic, c.cfg.QoS, func(_ paho.Client, msg paho.Message) {
			h(msg.Topic(), msg.Payload())
		})
	}
}

// Publish sends bytes to a topic.
func (c *Client) Publish(ctx context.Context, topic string, payload []byte, retain bool) error {
	if c == nil || c.client == nil {
		return platform.Unavailable("mqtt.disconnected", "MQTT is not connected")
	}
	if topic == "" || strings.ContainsAny(topic, "+#") {
		return platform.Invalid("mqtt.topic", "a concrete topic is required")
	}
	token := c.client.Publish(topic, c.cfg.QoS, retain, payload)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return platform.Unavailable("mqtt.timeout", "MQTT publish timed out")
	case ok := <-waitToken(token):
		if !ok {
			return platform.Wrap(platform.KindUnavailable, "mqtt.publish", "MQTT publish failed", token.Error())
		}
		return nil
	}
}

// Subscribe registers a handler and keeps it across reconnects.
func (c *Client) Subscribe(ctx context.Context, topic string, handler func(topic string, payload []byte)) error {
	if c == nil || c.client == nil {
		return platform.Unavailable("mqtt.disconnected", "MQTT is not connected")
	}
	c.mu.Lock()
	c.subs[topic] = handler
	c.mu.Unlock()
	token := c.client.Subscribe(topic, c.cfg.QoS, func(_ paho.Client, msg paho.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return platform.Unavailable("mqtt.timeout", "MQTT subscribe timed out")
	case ok := <-waitToken(token):
		if !ok {
			return platform.Wrap(platform.KindUnavailable, "mqtt.subscribe", "MQTT subscribe failed", token.Error())
		}
		return nil
	}
}

// Connected reports broker connectivity.
func (c *Client) Connected() bool {
	return c != nil && c.client != nil && c.client.IsConnected()
}

// Close disconnects.
func (c *Client) Close() {
	if c != nil && c.client != nil {
		c.client.Disconnect(250)
	}
}

func (c *Client) topic(prefix string, parts ...string) string {
	base := strings.Trim(c.cfg.TopicPrefix, "/")
	if base == "" {
		base = "aios"
	}
	all := append([]string{base}, parts...)
	if prefix != "" {
		all = append([]string{prefix}, all...)
	}
	return strings.Join(all, "/")
}

var _ = fmt.Sprint
