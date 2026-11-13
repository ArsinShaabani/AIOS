# Device Integration

AIOS connects ESP32, Raspberry Pi, STM32, Arduino, ROS2 robots, MQTT smart-home
nodes and HTTP firmware. Every device has a token, declared capabilities and
validated commands.

## 1. Register

```bash
curl -s -X POST http://localhost:8080/api/v1/devices \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Kitchen light","kind":"light","protocol":"mqtt","room":"Kitchen",
       "capabilities":[{"name":"power","type":"switch"},
                       {"name":"brightness","type":"number","min":0,"max":100}]}'
```

The response contains a one-time `token` (`dev_…`). Only a bcrypt hash and lookup
prefix are stored. Rotate with `POST /api/v1/devices/{id}/rotate-token` (the old
token stops working immediately).

Capabilities are the safety contract: `number` values outside min/max are rejected,
`enum` must match, `switch` accepts booleans/on/off. Robotics commands cannot exceed
these limits even if a model asks.

## 2. MQTT contract

Prefix `aios` (`mqtt.topic_prefix`):

| Topic | Direction | Payload |
|---|---|---|
| `aios/devices/<id>/cmd` | server → device | `{"request_id","device_id","capability","value","issued_at","expires_at"}` |
| `aios/devices/<id>/telemetry` | device → server | `{"metric","value","unit","at"}` |
| `aios/devices/<id>/status` | device → server | `online`/`offline` (use as LWT) |
| `aios/users/<user>/notifications` | server → client | reminder/automation events |

Override per device with metadata `command_topic` / `telemetry_topic` (e.g.
Zigbee2MQTT, Tasmota, Shelly, Sonoff). QoS 1, auto re-subscribe, presence cached
24 h.

### ESP32 minimal sketch

```cpp
#include <WiFi.h>
#include <PubSubClient.h>
#include <ArduinoJson.h>
WiFiClient net; PubSubClient mqtt(net);
const char* ID = "dev_xxx";

void onCommand(char*, byte* payload, unsigned int len) {
  StaticJsonDocument<256> doc; deserializeJson(doc, payload, len);
  if (strcmp(doc["capability"], "power") == 0) digitalWrite(LED_BUILTIN, doc["value"].as<bool>());
  mqtt.publish(("aios/devices/" + String(ID) + "/telemetry").c_str(),
               "{\"metric\":\"power\",\"value\":true}");
}
void setup() {
  WiFi.begin("ssid", "password");
  mqtt.setServer("192.168.1.10", 1883); mqtt.setCallback(onCommand);
  mqtt.connect(ID);
  mqtt.subscribe(("aios/devices/" + String(ID) + "/cmd").c_str(), 1);
}
void loop() { mqtt.loop(); }
```

## 3. HTTP firmware

Register with `"protocol":"http"` and `metadata.command_url`. AIOS POSTs the same
envelope there and parses `{"ok":true,"detail":...}` or plain text. Telemetry:

```bash
curl -s -X POST http://aios:8080/api/v1/device/telemetry \
  -H "Authorization: Bearer dev_…" -H 'Content-Type: application/json' \
  -d '{"metric":"temperature","value":21.5,"unit":"C"}'
```

Heartbeat: `POST /api/v1/device/heartbeat`. Identify: `GET /api/v1/device/me`.

## 4. Raspberry Pi

```bash
mosquitto_sub -h 192.168.1.10 -t "aios/devices/dev_pi/cmd" -v
mosquitto_pub -h 192.168.1.10 -t "aios/devices/dev_pi/telemetry" \
  -m '{"metric":"cpu_temp","value":48.2,"unit":"C"}'
```

Wrap GPIO in a small service mapping `capability` → pin action and publish telemetry
periodically. Declare capabilities (`gpio17`, `fan`, `camera_pan`) so the model
cannot improvise.

## 5. STM32 / bare metal

Use an ESP32 or TCP–MQTT bridge. The JSON envelope is ≤512 bytes and fits
constrained parsers. Keep the same ack/telemetry topics so AIOS needs no special
case.

## 6. ROS2

Bridge the command topic to ROS2:

- `cmd` → `geometry_msgs/Twist` for `move`/`rotate` (bounded m/s or rad/s);
- odometry/battery → `telemetry`;
- always declare an emergency-stop capability (`enum: ["stop","clear"]`).

`protocol: "rosbridge"` is reserved for a direct rosbridge WebSocket transport
(roadmap); MQTT is the supported v1 path.

## 7. Smart home

- **Home Assistant:** enable `home_assistant` in config; the tool supports
  `list_states`, `get_state`, `call_service`.
- **Zigbee2MQTT / Tasmota / Shelly / Sonoff:** register as MQTT devices with native
  topics.
- **Matter / Tuya Cloud:** expose through Home Assistant (recommended) or a plugin;
  direct cloud APIs are not supported in v1.

## 8. Commands, presence, safety

```bash
curl -s -X POST localhost:8080/api/v1/devices/dev_x/command \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"capability":"brightness","value":40}'
```

Errors: unknown capability/out-of-bounds → 400, offline transport → 503, unknown
device → 404. Every command is audited with actor, capability and outcome.
Presence: `GET /api/v1/devices?status=online`; cache:
`GET /api/v1/devices/{id}/telemetry`.

For thousands of devices keep `queue.workers` low and let MQTT fan out; scale out
with Redis and multiple AIOS replicas behind the broker.
