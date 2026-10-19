BudMinder integrates directly with AC Infinity cloud controllers using your account credentials, operating seamlessly alongside the official mobile app. It logs climate sensors (temperature, humidity, VPD and CO2) and records output power levels across all device ports without modifying your controller programming or trigger rules.

## Connecting

1. Go to **Settings → Integrations → AC Infinity** and toggle on **Enable AC Infinity Sensor Monitoring**.
1. Click **Retrieve Token** and log in with your AC Infinity account email and password.
1. Check **Stay signed in** (recommended; see details below) and complete the sign-in. BudMinder confirms the active connection, and live readings will populate within one minute.
1. Navigate to [Manage sensors](/manual/manage-sensors), click **Scan AC Infinity**, assign the detected devices to your tents and click **Add**.

The **Polling Interval** setting on the same integration card controls data refresh frequency: configurable between 15 and 300 seconds (defaulting to 60 seconds).

> **Tip:** The AC Infinity authentication server only evaluates the first 25 characters of account passwords. If your password is longer, simply enter it completely as normal.

## Stay signed in

AC Infinity sessions naturally expire periodically. Without **Stay signed in** enabled, telemetry collection pauses when an authentication token expires, prompting a **Sign in again** warning banner on your Today dashboard.

When **Stay signed in** is enabled, BudMinder securely stores your encrypted password and re-authenticates automatically whenever a session token expires. The decryption key is isolated in a standalone local file within BudMinder's storage directory (never written to the main database or included in standard backups). Click **Forget saved password** at any time to disable automatic re-authentication.

## Ports

Controller device ports (inline fans, oscillating fans, grow lights and humidifiers) register as discrete sensors, tracking active power percentage or displaying **Off** when idle. Assign descriptive port labels matching connected equipment. When a port name includes lighting keywords (such as *LED*, *Light* or *Mars Hydro FC3000*):

- the Today dashboard displays real-time light status (On/Off) for that tent;
- new smoke reports created for plants in that tent automatically suggest that hardware as the lighting source.

## If readings stop

- Check the AC Infinity status card under [Manage sensors](/manual/manage-sensors) to see if the session has expired or the server is unreachable.
- Use **Troubleshooting: view the raw AC Infinity data** at the bottom of the Manage sensors page to inspect live API communication. Ensure **Hide personal details** remains checked if sharing diagnostic logs.
