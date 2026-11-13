package tools

import (
	"github.com/ArsinShaabani/AIOS/internal/config"
)

// Deps bundles the services tools depend on. Nil fields simply skip the
// corresponding tool, which keeps a minimal install free of dead capabilities.
type Deps struct {
	Config    config.Config
	Devices   DeviceAPI
	Memory    MemoryAPI
	Scheduler SchedulerAPI
	MQTT      MQTTPublisher
	Home      HomeAssistantAPI
	Email     EmailSender
}

// BuildAll registers every available built-in tool.
func BuildAll(reg *Registry, deps Deps) error {
	web := NewWebTools(deps.Config.Tools, nil)
	if err := web.Register(reg); err != nil {
		return err
	}
	if err := RegisterKnowledgeTool(reg, web.client); err != nil {
		return err
	}
	if err := RegisterDeviceTools(reg, deps.Devices); err != nil {
		return err
	}
	if err := RegisterMemoryTools(reg, deps.Memory); err != nil {
		return err
	}
	if err := RegisterSchedulerTools(reg, deps.Scheduler); err != nil {
		return err
	}
	if err := RegisterHomeAssistantTool(reg, deps.Home); err != nil {
		return err
	}
	if err := RegisterEmailTool(reg, deps.Email); err != nil {
		return err
	}
	if err := RegisterNavigationTool(reg, web.client); err != nil {
		return err
	}
	return RegisterMQTTTool(reg, deps.MQTT)
}
