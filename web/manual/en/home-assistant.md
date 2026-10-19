If you already monitor climate devices through Home Assistant, BudMinder can import telemetry from any supported entity: including Govee, SwitchBot, Inkbird, Zigbee, ESPHome or custom integrations reporting numeric values (temperature, humidity, CO2, soil moisture, light levels or electrical conductivity). BudMinder operates strictly in read-only mode and never alters your Home Assistant states or entity configurations.

## Connecting

1. In Home Assistant, create a dedicated non-administrator user account for BudMinder (**Settings → People → Add person**, enabling login access).
1. Log in to Home Assistant using that account, navigate to user profile → **Security** → **Long-lived access tokens**, and generate a new token. Copy the token immediately, as Home Assistant displays it only once.
1. In BudMinder, go to **Settings → Integrations → Home Assistant** and toggle on **Read sensors from Home Assistant**.
1. Enter your Home Assistant network **Address** and port (for example, *http://192.168.1.20:8123*), then paste your **Access token**.
1. Click **Test connection**. BudMinder validates the link and reports total detected entities alongside how many are cultivation-relevant.
1. Navigate to [Manage sensors](/manual/manage-sensors), click **Scan Home Assistant**, select your desired devices and map each to a tent zone.

Once saved, access tokens are masked and cannot be viewed again.

## Choosing sensors

Device scans filter and prioritize cultivation-relevant sensors, displaying active readings alongside entity names. Use the **Filter found sensors** field to search specific devices. Non-environmental entities (like battery percentages and network monitors) remain hidden until you enable **Show all**. Click **Add these** to import all currently filtered entities at once.

## If readings stop

If Home Assistant restarts or temporarily goes offline, BudMinder automatically resumes polling as soon as connectivity restores. The Home Assistant card under Manage sensors displays **Not responding** if the host is unreachable, or **Token refused** if the authentication token has been revoked or modified. If revoked, generate a replacement token and update your settings.
