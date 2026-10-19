BudMinder reads AC Infinity controllers through your AC Infinity account, the same way the AC Infinity app does. It reads the controller's sensors, such as temperature, humidity, VPD and CO₂, and the power level of each port. It never changes your controller's settings.

## Connecting

1. Go to **Settings → Integrations → AC Infinity** and turn on **Enable AC Infinity Sensor Monitoring**.
2. Click **Retrieve Token** and sign in with your AC Infinity email and password.
3. Tick **Stay signed in** (recommended, see below), then sign in. BudMinder says when it's signed in, and readings start within a minute.
4. Go to [Manage sensors](/manual/manage-sensors), click **Scan AC Infinity**, put the sensors in your tents and click **Add**.

**Polling Interval**, in the same card, sets how often BudMinder fetches new readings: every 15 to 300 seconds, 60 by default.

> **Tip:** AC Infinity's sign-in only uses the first 25 characters of a password. If yours is longer, that's normal; type it in full.

## Stay signed in

AC Infinity logs BudMinder out from time to time. Without **Stay signed in**, readings stop until you sign in again, and the Today page warns you with a **Sign in again** button.

With **Stay signed in** ticked, BudMinder keeps your AC Infinity password, encrypted, and signs in again by itself when that happens. The key that unlocks it is kept in its own file in BudMinder's data folder, not in the database and not in backups. Click **Forget saved password** to turn it off.

## Ports

Each port on the controller (a fan, a light, a humidifier) is added as a sensor too, showing its power level, or **Off**. Name each port after what's plugged into it. When a port's name looks like a light, such as *LED* or *Mars Hydro FC3000*:

- the Today page shows whether the tent's lights are on or off;
- a new smoke report for a plant in that tent suggests it as the light.

## If readings stop

- Check the AC Infinity card on [Manage sensors](/manual/manage-sensors). It says if the sign-in expired or AC Infinity isn't responding.
- **Troubleshooting: view the raw AC Infinity data**, at the bottom of Manage sensors, shows exactly what AC Infinity sends. Keep **Hide personal details** ticked before sharing it with anyone.
