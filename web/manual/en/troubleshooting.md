Common issues and step-by-step solutions. If an issue is not listed here, consult the application logs below for detailed diagnostic messages.

## I forgot my password

BudMinder does not have an external password recovery or email reset system. If you maintain an active session on another browser or device (such as via **Remember Me**), open **Settings → General → Password** there and enter a new password.

## AC Infinity readings stopped

If the Today dashboard warns that **AC Infinity refused BudMinder's sign-in**, navigate to **Settings → Integrations → AC Infinity**, click **Retrieve Token** and re-enter your account credentials. Check **Stay signed in** so BudMinder can renew authentication tokens automatically in the future. See [AC Infinity](/manual/ac-infinity#stay-signed-in).

## Ecowitt or Home Assistant readings stopped

Inspect the connection status cards under [Manage sensors](/manual/manage-sensors). A status of **Not responding** indicates that BudMinder cannot reach the host: verify the device is powered on, connected to your network and that its IP address has not changed. A status of **Token refused** (Home Assistant) indicates an invalid or revoked access token; generate a fresh token in Home Assistant and paste it into BudMinder settings.

## A sensor says when its last reading was

This warning indicates that the sensor stopped transmitting fresh telemetry. Check the physical device (power supply, battery level, Wi-Fi or Zigbee signal strength) and review its parent integration status under Manage sensors.

## Dates or times are off

Verify your **Timezone** configuration under **Settings → General**. Without a configured timezone, BudMinder defaults to the host server's local clock, which defaults to UTC in many Docker and Linux installations.

## Guest view didn't change

Toggling guest access on or off requires a full restart of the BudMinder application to reconfigure web routing permissions.

## There's no Live button for my camera

Only camera feeds using HLS stream URLs (ending in `.m3u8`) support interactive in-browser live streaming. See [Tent cameras](/manual/tent-cameras#which-addresses-work).

## A strain lookup doesn't work

Verify that **StrainCompass Strain Import** is enabled under **Settings → Integrations**, and ensure your host server has outbound internet connectivity. Public StrainCompass searches also enforce daily rate limits if used without a personal API key.

## The harvest estimate says Not set

This status appears when the cultivar lacks flowering duration data in your strain library, or when a photoperiod plant has not yet been advanced to Flower on its stage track. Add the flowering duration under **Edit** on the strain profile. See [Plant page](/manual/plant-page#growth-stages).

## The logs

**Settings → Logs** provides real-time system visibility into BudMinder operations.

- **App Log** records internal application events and errors; **Access Log** records page views and HTTP requests.
- Adjust **How many lines** to load, filter logs to display only **Warnings** or **Errors**, or use the search field to locate specific keywords.
- Click **Download** to export the active log file for troubleshooting or community support.

The **Log Level** setting under **Log detail** controls verbosity. **Info** is recommended for standard daily use. Switch to **Debug** while investigating a technical issue, then return to Info once resolved. Log level changes take effect immediately without restarting.
