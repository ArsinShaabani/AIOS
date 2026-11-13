package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type geoPoint struct {
	Lat   float64
	Lon   float64
	Label string
}

func geocode(ctx context.Context, client *WebClient, query string) (geoPoint, error) {
	endpoint := "https://nominatim.openstreetmap.org/search?format=json&limit=1&q=" + url.QueryEscape(query)
	body, _, err := client.Get(ctx, endpoint, 1<<20)
	if err != nil {
		return geoPoint{}, err
	}
	var rows []struct {
		Lat     string `json:"lat"`
		Lon     string `json:"lon"`
		Display string `json:"display_name"`
	}
	if err := json.Unmarshal(body, &rows); err != nil || len(rows) == 0 {
		return geoPoint{}, platform.NotFound("nav.geocode", "could not geocode: "+query)
	}
	var lat, lon float64
	fmt.Sscanf(rows[0].Lat, "%f", &lat)
	fmt.Sscanf(rows[0].Lon, "%f", &lon)
	return geoPoint{Lat: lat, Lon: lon, Label: rows[0].Display}, nil
}

// RegisterNavigationTool adds navigate_route using OSM Nominatim + OSRM.
func RegisterNavigationTool(reg *Registry, client *WebClient) error {
	if client == nil {
		return nil
	}
	return reg.Register(ports.Tool{
		Name:        "navigate_route",
		Description: "Geocode two places and return straight route distance, duration and an OpenStreetMap link.",
		Parameters: Schema(map[string]any{
			"origin":      map[string]any{"type": "string", "description": "Starting place"},
			"destination": map[string]any{"type": "string", "description": "Destination place"},
			"mode":        map[string]any{"type": "string", "enum": []string{"driving", "walking", "cycling"}, "default": "driving"},
		}, "origin", "destination"),
		Permissions: []string{PermInternet},
		Timeout:     30,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			origin := stringArg(call.Arguments, "origin")
			destination := stringArg(call.Arguments, "destination")
			if origin == "" || destination == "" {
				return ports.ToolResult{IsError: true, Content: "origin and destination are required"}, nil
			}
			mode := stringArg(call.Arguments, "mode")
			if mode == "" {
				mode = "driving"
			}
			start, err := geocode(ctx, client, origin)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			end, err := geocode(ctx, client, destination)
			if err != nil {
				return ports.ToolResult{IsError: true, Content: err.Error()}, nil
			}
			link := fmt.Sprintf("https://www.openstreetmap.org/directions?engine=fossgis_osrm_%s&route=%.5f,%.5f;%.5f,%.5f",
				mode, start.Lat, start.Lon, end.Lat, end.Lon)
			out := map[string]any{
				"origin":      start.Label,
				"destination": end.Label,
				"map_url":     link,
			}
			// OSRM public demo server; failures still return the map link.
			osrm := fmt.Sprintf("https://router.project-osrm.org/route/v1/%s/%.5f,%.5f;%.5f,%.5f?overview=false",
				mode, start.Lon, start.Lat, end.Lon, end.Lat)
			if body, _, err := client.Get(ctx, osrm, 1<<20); err == nil {
				var route struct {
					Routes []struct {
						Distance float64 `json:"distance"`
						Duration float64 `json:"duration"`
					} `json:"routes"`
				}
				if json.Unmarshal(body, &route) == nil && len(route.Routes) > 0 {
					out["distance_km"] = route.Routes[0].Distance / 1000
					out["duration_min"] = route.Routes[0].Duration / 60
				}
			}
			raw, _ := json.Marshal(out)
			return ports.ToolResult{Content: string(raw), Data: out}, nil
		},
	})
}

// RegisterMQTTTool adds mqtt_publish for raw IoT topics.
func RegisterMQTTTool(reg *Registry, pub MQTTPublisher) error {
	if pub == nil {
		return nil
	}
	return reg.Register(ports.Tool{
		Name:        "mqtt_publish",
		Description: "Publish a message to an MQTT topic (advanced IoT integration). Use device_command when a device is registered.",
		Parameters: Schema(map[string]any{
			"topic":   map[string]any{"type": "string", "description": "MQTT topic"},
			"payload": map[string]any{"type": "string", "description": "Message payload"},
			"retain":  map[string]any{"type": "boolean", "default": false},
		}, "topic", "payload"),
		Permissions: []string{"devices:write"},
		Timeout:     15,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			topic := stringArg(call.Arguments, "topic")
			payload := stringArg(call.Arguments, "payload")
			if topic == "" {
				return ports.ToolResult{IsError: true, Content: "topic is required"}, nil
			}
			if strings.Contains(topic, "#") || strings.Contains(topic, "+") {
				return ports.ToolResult{IsError: true, Content: "wildcards are not allowed when publishing"}, nil
			}
			retain, _ := call.Arguments["retain"].(bool)
			if err := pub.Publish(ctx, topic, []byte(payload), retain); err != nil {
				return ports.ToolResult{IsError: true, Content: "publish failed: " + err.Error()}, nil
			}
			return ports.ToolResult{Content: "published to " + topic}, nil
		},
	})
}
