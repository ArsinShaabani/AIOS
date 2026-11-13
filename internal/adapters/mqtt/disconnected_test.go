package mqtt

import (
	"context"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestDisconnectedTransport(t *testing.T) {
	var c *Client
	if err := c.Publish(context.Background(), "x", []byte("y"), false); err == nil {
		t.Fatal("nil client publish should fail")
	}
	if c.Connected() {
		t.Fatal("nil client connected")
	}
	tr := NewTransport(c, "aios")
	res, err := tr.Send(context.Background(), domain.Device{ID: "dev1"}, ports.Command{
		RequestID: "r1", Capability: "power", Value: true,
	})
	if err == nil {
		t.Fatal("send without a broker should fail")
	}
	if res.OK {
		t.Fatal("failed send reported success")
	}
	_ = config.MQTTConfig{}
}
