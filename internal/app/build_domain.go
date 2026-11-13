package app

import (
	"context"
	"encoding/json"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpdev"
	"github.com/ArsinShaabani/AIOS/internal/adapters/mqtt"
	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/devices"
	"github.com/ArsinShaabani/AIOS/internal/mqttbridge"
	"github.com/ArsinShaabani/AIOS/internal/plugins"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/scheduler"
	"github.com/ArsinShaabani/AIOS/internal/tools"
	"github.com/ArsinShaabani/AIOS/internal/voice"
)

func (a *App) initDomain(ctx context.Context) error {
	transports := []ports.DeviceTransport{httpdev.New()}
	if a.Config.MQTT.Enabled {
		client, err := mqtt.Connect(ctx, a.Config.MQTT, a.Log)
		if err != nil {
			a.Log.Warn("MQTT disabled: broker unreachable", "error", err)
		} else {
			a.MQTT = client
			transport := mqtt.NewTransport(client, a.Config.MQTT.TopicPrefix)
			transports = append(transports, transport)
			a.MQTTTransport = transport
		}
	}
	a.Devices = devices.New(a.Repos.Devices, transports, a.Cache, a.Auditor, a.Bus,
		a.Config.Security.BcryptCost, a.Log)
	a.Scheduler = scheduler.New(a.Repos.Tasks, a.Queue, a.Config.Scheduler, a.Log)

	registry := tools.NewRegistry()
	var haAPI tools.HomeAssistantAPI
	if a.Config.HomeAssistant.Enabled {
		haAPI = tools.NewHomeAssistant(a.Config.HomeAssistant)
	}
	deps := tools.Deps{
		Config: a.Config, Devices: deviceAPI{svc: a.Devices}, Memory: memoryAPI{svc: a.Memory},
		Scheduler: a.Scheduler, MQTT: a.mqttPublisher(), Home: haAPI, Email: a.buildEmail(ctx),
	}
	if err := tools.BuildAll(registry, deps); err != nil {
		return err
	}
	a.Tools = registry
	a.Brain = &brain.Engine{
		Users: a.Repos.Users, Sessions: a.Repos.Sessions, Messages: a.Repos.Messages,
		Memory: a.Memory, Personalities: a.Personality, Agents: a.Agents, Tools: registry,
		Router: a.Router, Queue: a.Queue, Events: a.Bus, Audit: a.Auditor,
		Config: a.Config, Builder: brain.NewBuilder(a.Memory, a.Repos.Messages, a.Config.Memory, a.Config.LLM.SystemPrompt),
		Log: a.Log,
	}
	stt, tts, err := voice.BuildEngines(a.Config.Voice)
	if err != nil {
		if a.Config.Voice.Enabled {
			return err
		}
		a.Log.Warn("voice engines unavailable", "error", err)
		stt, tts = voice.MockSTT{}, voice.MockTTS{}
	}
	a.Voice = voice.NewService(a.Config.Voice, stt, tts, a.Log)
	a.Plugins = plugins.New(a.Repos.Plugins, registry, a.Config.Plugins, a.Log)
	if a.MQTT != nil && a.Config.MQTT.Bridge.Enabled {
		a.MQTTBridge = mqttbridge.New(a.Config.MQTT, a.MQTT, a.Repos.Tokens, a.Repos.Users, a.Brain, a.Auditor, a.Log)
	}
	return nil
}

func (a *App) mqttPublisher() tools.MQTTPublisher {
	if a.MQTTTransport == nil {
		return nil
	}
	return a.MQTTTransport
}

func (a *App) buildEmail(ctx context.Context) tools.EmailSender {
	row, err := a.Repos.Settings.Get(ctx, "global", "", "smtp")
	if err != nil {
		return nil
	}
	var cfg tools.SMTPConfig
	if json.Unmarshal(row.Value, &cfg) != nil {
		return nil
	}
	if secret, err := a.Secrets.Get(ctx, "smtp.password"); err == nil {
		cfg.Password = secret
	}
	sender := tools.NewSMTP(cfg)
	if sender == nil {
		return nil
	}
	return sender
}
