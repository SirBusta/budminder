If you develop custom DIY hardware (such as an ESP32 or Raspberry Pi running environmental probes), your devices can transmit readings directly into BudMinder using the built-in REST API. Custom sensor feeds appear seamlessly across the Environment and Today dashboards alongside commercial hardware.

## The API key

1. Navigate to **Settings → Integrations → Sensor API** and click to generate an **API Key**.
1. Copy the key immediately. For security, it is displayed only once.

Generating a new key instantly invalidates the prior key, requiring updates across your transmitting hardware. To pause incoming transmissions temporarily without resetting credentials, toggle **Disable API ingest** on the same card.

## Sending a reading

Configure your microcontroller to transmit telemetry via an HTTP `POST` request to your BudMinder server URL followed by `/api/sensors/ingest`. Include the API key in the `X-API-KEY` request header and send the sensor payload as JSON:

```json
{
  "source": "custom",
  "device": "ESP32 tent probe",
  "type": "temperature",
  "value": 25.5,
  "name": "Canopy temp",
  "new_zone": "Tent 1",
  "unit": "°C"
}
```

- `source`, `device` and `type` combine to form a unique hardware fingerprint. The initial payload received with a new combination automatically provisions the sensor.
- `value` contains the numeric measurement.
- `name`, `unit` and `new_zone` define default labels and tent assignments when the sensor is first provisioned.

Newly provisioned devices appear under [Manage sensors](/manual/manage-sensors), where you can modify names, reassign zones and configure display visibility.

> **Tip:** The ingestion endpoint enforces a rate limit of up to 60 readings per minute. Transmitting once per minute across multiple sensors works smoothly, while rapid high-frequency transmissions exceeding the limit will be rejected.

## Reading data back out

The same API key enables authenticated `GET /api/overlay` requests, returning a structured JSON payload of all active plants and current sensor metrics. This endpoint is designed specifically for automated live-stream HUD overlays (such as OBS browser sources).
