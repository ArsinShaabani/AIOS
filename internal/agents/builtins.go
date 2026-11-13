package agents

import (
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Builtins returns the shipped agent catalogue. Core tools are available to the
// general agent; specialists get narrower, cheaper tool sets.
func Builtins() []domain.Agent {
	core := []string{"web_search", "web_fetch", "memory_search", "memory_save", "document_search", "schedule_reminder", "device_list", "device_command", "home_assistant"}
	return []domain.Agent{
		{ID: "agent_general", Name: "General", Description: "General assistant with tools for search, memory, devices and reminders.",
			SystemPrompt: "You are the general assistant. Answer directly; use a tool when it makes the answer more accurate.",
			Tools:        core, Keywords: []string{"help", "assistant", "کمک", "دستیار"}, Priority: 1, Enabled: true, Builtin: true},
		{ID: "agent_search", Name: "Search", Description: "Web research, page reading, comparison and fact checking.",
			SystemPrompt: "You are a research agent. Search the web, read the most relevant pages, cite URLs and clearly separate facts from inference. If sources disagree, say so.",
			Tools:        []string{"web_search", "web_fetch", "knowledge_lookup", "memory_save", "document_search"},
			Keywords:     []string{"search", "google", "news", "research", "compare", "who is", "what is", "latest", "جستجو", "تحقیق", "خبر", "آخرین"},
			Priority:     5, Enabled: true, Builtin: true},
		{ID: "agent_home", Name: "Home", Description: "Smart-home control through Home Assistant and local devices.",
			SystemPrompt: "You are a smart-home agent. Prefer Home Assistant for lights, switches, scenes and climate. Confirm destructive actions before executing them.",
			Tools:        []string{"home_assistant", "device_list", "device_command", "mqtt_publish"},
			Keywords:     []string{"light", "lamp", "switch", "kitchen", "bedroom", "living room", "turn on", "turn off", "thermostat", "scene", "home assistant", "چراغ", "لامپ", "ترموستات", "صحنه", "خانه", "اتاق"},
			Priority:     6, Enabled: true, Builtin: true},
		{ID: "agent_robotics", Name: "Robotics", Description: "Robot motion, telemetry and safety-bounded commands.",
			SystemPrompt: "You are a robotics agent. Commands must respect capability limits. Never command movement without a clear target and a stop condition; warn about safety.",
			Tools:        []string{"device_list", "device_command", "device_telemetry"},
			Keywords:     []string{"robot", "move", "forward", "backward", "turn left", "turn right", "rotate", "gripper", "ros2", "ربات", "حرکت", "جلو", "چرخش", "توقف"},
			Priority:     8, Enabled: true, Builtin: true},
		{ID: "agent_iot", Name: "IoT", Description: "Sensors, actuators and MQTT endpoints.",
			SystemPrompt: "You are an IoT agent. Read sensor state before acting, prefer explicit device IDs and report units.",
			Tools:        []string{"device_list", "device_command", "device_telemetry", "mqtt_publish"},
			Keywords:     []string{"sensor", "temperature", "humidity", "relay", "esp32", "arduino", "stm32", "mqtt", "سنسور", "دما", "رطوبت", "رله"},
			Priority:     7, Enabled: true, Builtin: true},
		{ID: "agent_scheduler", Name: "Scheduler", Description: "Reminders, timers and recurring automations.",
			SystemPrompt: "You are a scheduling agent. Convert relative times into absolute timestamps, confirm the timezone and summarise what will happen and when.",
			Tools:        []string{"schedule_reminder", "list_reminders", "schedule_prompt"},
			Keywords:     []string{"remind", "reminder", "timer", "alarm", "schedule", "tomorrow", "یادآوری", "تایمر", "زنگ", "فردا", "قرار"},
			Priority:     6, Enabled: true, Builtin: true},
		{ID: "agent_email", Name: "Email", Description: "Compose and send email via SMTP.",
			SystemPrompt: "You are an email agent. Draft concise, professional messages, confirm recipients, and never send without an explicit instruction.",
			Tools:        []string{"send_email", "web_search"},
			Keywords:     []string{"email", "mail", "send a message", "ایمیل", "نامه", "پیام"},
			Priority:     5, Enabled: true, Builtin: true},
		{ID: "agent_navigation", Name: "Navigation", Description: "Routes, distances and map links.",
			SystemPrompt: "You are a navigation agent. Provide distance, duration and a maps link. Ask for an origin when it is missing.",
			Tools:        []string{"navigate_route"},
			Keywords:     []string{"route", "navigate", "directions", "distance", "مسیر", "ناوبری", "فاصله", "جهت"},
			Priority:     5, Enabled: true, Builtin: true},
		{ID: "agent_device", Name: "Devices", Description: "Device inventory, health and diagnostics.",
			SystemPrompt: "You are a device management agent. Report online state, last seen times and capabilities; suggest fixes for offline devices.",
			Tools:        []string{"device_list", "device_telemetry", "device_command"},
			Keywords:     []string{"device", "devices", "offline", "firmware", "battery", "دستگاه", "سخت افزار", "باتری"},
			Priority:     4, Enabled: true, Builtin: true},
		{ID: "agent_automation", Name: "Automation", Description: "Create delayed or recurring prompts and actions.",
			SystemPrompt: "You are an automation agent. Build automations from clear triggers and actions, confirm them, and keep them reversible.",
			Tools:        []string{"schedule_prompt", "list_reminders", "device_command", "home_assistant"},
			Keywords:     []string{"automate", "automation", "every day", "when", "اتوماسیون", "خودکار", "هر روز", "وقتی"},
			Priority:     4, Enabled: true, Builtin: true},
		{ID: "agent_knowledge", Name: "Knowledge", Description: "Encyclopaedic answers with citations.",
			SystemPrompt: "You are a knowledge agent. Use Wikipedia and web sources, cite them, and state uncertainty honestly.",
			Tools:        []string{"knowledge_lookup", "web_fetch", "document_search"},
			Keywords:     []string{"history", "define", "meaning", "encyclopedia", "تاریخ", "معنی", "تعریف", "دانشنامه"},
			Priority:     3, Enabled: true, Builtin: true},
	}
}

// Validate checks an agent definition submitted through the API.
func Validate(a *domain.Agent) error {
	if strings.TrimSpace(a.Name) == "" {
		return platform.Invalid("agent.name", "agent name is required")
	}
	if strings.TrimSpace(a.SystemPrompt) == "" {
		return platform.Invalid("agent.prompt", "agent system prompt is required")
	}
	if len(a.Tools) > 32 {
		return platform.Invalid("agent.tools", "an agent may expose at most 32 tools")
	}
	return nil
}
