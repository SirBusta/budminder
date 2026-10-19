If you build your own sensors, for example an ESP32 board with a temperature probe, they can send their readings to BudMinder. Their readings then show on the Environment and Today pages like any other sensor.

## The API key

1. Go to **Settings → Integrations → Sensor API** and generate an **API Key**.
2. Copy it straight away. It's shown only once.

Generating a new key replaces the old one at once, so update your devices when you do. **Disable API ingest**, in the same card, blocks outside readings for a while without removing the key.

## Sending a reading

Your device sends each reading to BudMinder's address followed by `/api/sensors/ingest`, as a `POST` request with the key in an `X-API-KEY` header and the reading as JSON:

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

- `source`, `device` and `type` together identify the sensor. The first reading with a new combination creates the sensor.
- `value` is the reading.
- `name`, `unit` and `new_zone` (the tent to put it in) are used when the sensor is created.

New sensors appear in [Manage sensors](/manual/manage-sensors), where you can rename them, move them to another tent and choose where they show.

> **Tip:** BudMinder accepts up to 60 readings a minute. Several sensors each sending once a minute is fine; a device sending many times a second will be turned away.

## Reading data back out

The same key also allows `GET /api/overlay`, which returns every growing plant and every sensor's latest reading. It's made for live-stream overlays, such as an OBS browser source.
