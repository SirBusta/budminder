BudMinder sends you an alert notification when a zone's climate, lights or sensors need attention. Manage alert notifications in **Settings → Alerts**.

> **Beta.** Alerts are still being tested. Do not rely on them as your only safety check. Feedback is appreciated: [budminder.app@gmail.com](mailto:budminder.app@gmail.com?subject=BudMinder%20alerts%20feedback).

> **Defaults are a starting point.** Adjust the Targets and limits for your grow space in **Settings → Grow setup** → [Climate targets and limits](/manual/grow-setup#climate-targets-and-limits). If you receive too many alert notifications, widen your limits.

## What sends an alert notification

| Alert | Sent when |
|---|---|
| **Climate (VPD, Temp & Humidity)** | Climate reading reaches a Low or High limit for 15 consecutive minutes |
| **Lights did not turn on or off** | Lights turn on or off outside the scheduled time for 15 consecutive minutes |
| **Lights are off schedule** | Lights switch about 1 hour before or after the schedule on each of the last 3 days |
| **Sensor alerts** | Sensor reaches a user defined value for 15 consecutive minutes |
| **Sensors offline** | No readings received for 15 minutes from a climate sensor, a light sensor or a sensor with a Sensor alert turned on |
| **Integration problem** | An integration refuses BudMinder's sign-in or access token, or does not respond, for 15 consecutive minutes |

Set limits in [Climate targets and limits](/manual/grow-setup#climate-targets-and-limits), in **Settings → Grow setup**. Temperature and humidity start with wide limits for each stage. VPD starts with Targets and no limits, so BudMinder sends no VPD alert notification until you set a limit.

BudMinder checks climate and lights only in zones with plants, and only with the sensors chosen in **Edit Zone**. A sensor that never sent a reading does not count as offline.

## How often alert notifications are sent

| When | What happens |
|---|---|
| **First 15 minutes** | Nothing is sent. One reading back in range resets the 15-minute count, so brief spikes are ignored. |
| **At 15 minutes** | The first notification is sent. |
| **Every 6 hours** | A reminder is sent until the problem ends. Each reminder lists every active alert. |
| **After 30 minutes back in range** | One final all-clear is sent. If a zone has several climate alerts, you receive one all-clear when the last one ends. |

**Lights are off schedule** is sent once, with no reminder or all-clear. It replaces **Lights did not turn on or off** while the lights stay about 1 hour off schedule.

- **Reaching a value counts.** A High limit of 1.50 kPa is reached at exactly 1.50.
- **BudMinder waits after lights turn on or off.** It ignores climate readings for the first 10 minutes, so a climate alert can start no sooner than 25 minutes after the change.
- **A sensor that stops sending readings** does not start or end a climate alert. After 15 minutes, BudMinder sends a **Sensors offline** alert notification.
- **When an integration stops working** for 15 minutes, BudMinder sends an **Integration problem** alert notification. The alert ends when the integration has worked normally for 30 minutes, or at once with no notification if you turn the integration off or sign out of it. If the integration's sensors are used for climate, Light check or Sensor alerts, the same notification usually also includes a **Sensors offline** alert.
- **Alerts that start together** arrive in one notification. If a notification method fails, BudMinder tries that method again every 5 minutes, and **Recent alerts** shows the error. Methods that worked do not receive the notification twice.
- **Alerts end without a notification** if you remove the limit, if you remove the sensor in **Edit Zone**, or if the zone has no plants left.

## Zone status

Shows each zone's current readings, limits and alerts, and the status of its lights and sensors. The bar for each reading shows these areas:

- **Target zone** (green): near the Target. If a limit is set, this is within half the distance from the Target to the limit.
- **Caution zone** (amber): between the Target zone and a limit. It appears only when a row has both a Target and a limit.
- **No target or High/Low limit set** (gray): no Target or limit applies to a reading here.
- **Alert zone** (red): at or past a limit.

The green mark is the Target. The other mark is the current reading. Reload the page to see new readings.

## Notification methods

BudMinder can send alert notifications by email, Pushover, the Home Assistant app, ntfy, Discord and a webhook. Each notification goes through every method that is turned on. Click a method to open its settings, fill them in, and send a test. When the test arrives, turn on the method's switch.

Before you can turn on a method, set a limit in **Climate targets and limits**, add a **Sensor alert**, or set up a **Light check** in **Edit Zone**. With every method off, alerts still appear under **Recent alerts**. Notifications use the language BudMinder was showing when you last changed a method's settings.

Passwords, tokens and webhook URLs are stored encrypted and are left out of backups. Saved values are not shown again. To keep one, leave its box blank.

In **BudMinder address**, below the list of methods, enter the address you use to open BudMinder, such as http://192.168.1.10:8080. Notifications link to that address. Tapping a phone notification opens the alert's readings on the Sensors page. The link opens only when your phone can reach that address, such as while connected to your home network or a VPN like Tailscale or WireGuard. Otherwise, the notification still arrives, but the page does not open. Phones cannot open a localhost address, such as http://localhost:8080, even at home. If BudMinder address shows one, replace it with the address other devices on your network use, such as http://192.168.1.10:8080.

- **Email.** Enter your **Mail server**, **Username**, **Password**, **From address** and **Send to** addresses. Gmail and most other mail services require an app password, not your account password.
- **Pushover.** An app for iPhone and Android, built for urgent notifications. It is a one-time purchase after a 30-day trial. Create an application for BudMinder on the Pushover website, then enter its **API token** and the **User key** from your Pushover dashboard.
- **Home Assistant app.** Set up the Home Assistant connection in **Settings → Integrations** first. Click **Refresh list**, then choose the phones where the Home Assistant app is signed in. When you sign in to the Android app, its "secure your remote connection" step may keep returning to the phone's location settings. Set the app's location permission to **Allow all the time**, or choose **Less secure** in that step.
- **ntfy.** A free app for iPhone and Android. Click **Generate** for a hard-to-guess topic, then subscribe to that topic in the ntfy app. On ntfy.sh, anyone who knows the topic can read your notifications. If you run your own ntfy server, enter its address in **Server**, and set `upstream-base-url` to https://ntfy.sh in the server's settings, or iPhones may receive notifications late or not at all. While the ntfy app is open on screen, notifications arrive without sound.
- **Discord.** In Discord, click **Edit Channel**, then **Integrations**. Click **Create Webhook**, or **Webhooks → New Webhook** if the channel already has webhooks. Then click **Copy Webhook URL** on the new webhook and paste the URL into **Webhook URL**. BudMinder messages never @mention anyone, so set the server's notifications to **All Messages**. Discord sends no phone notifications while the Discord app is running on your computer. Quit it there to get them on your phone. Discord notifications follow your phone's sound settings and cannot wake you.
- **Webhook.** For Node-RED, n8n and other services that accept a webhook. BudMinder sends each alert as JSON in a POST request. The `event` field is `start`, `reminder` or `end`, or `test` when you click **Send test request**.

### Wake me up

Pushover, the Home Assistant app and ntfy (Android only) can sound even when your phone is muted or on Do Not Disturb. Under **Alert notifications** in the method's settings, choose:

- **Regular**: sounds and vibrates like your other notifications. Silent when the phone is muted or on Do Not Disturb.
- **Wake me up**: sounds even when the phone is muted or on Do Not Disturb. Needs a one-time setup on the phone.
- **Wake me up and repeat** (Pushover only): also repeats every 5 minutes, for up to 3 hours, until you acknowledge it in the Pushover app.

The choice applies to the first notification of an alert and to its reminders. All-clear notifications always arrive as regular notifications.

When you choose Wake me up, setup steps for Android and iPhone appear in the method's settings:

- **Pushover on Android**: in the Pushover app, tap **⋮** at the top right, then **Settings**, and turn on **Play high-priority as alarm**. Wake me up notifications then play at your phone's alarm volume. They sound unless alarms are turned off in your phone's Do Not Disturb settings.
- **Pushover on iPhone**: in the Pushover app's **Settings**, turn on **Critical Alerts** for high priority (Wake me up) or emergency priority (Wake me up and repeat).
- **Home Assistant app on Android**: nothing to set up. The app plays Wake me up notifications at alarm volume. They sound unless alarms are turned off in your phone's Do Not Disturb settings.
- **Home Assistant app on iPhone**: in the iPhone's **Settings** app, go to **Notifications → Home Assistant** and turn on **Critical Alerts**.
- **ntfy on a Samsung phone**: in the phone's **Settings**, go to **Notifications → Do not disturb → App notifications → Add apps** and choose **ntfy**. The phone then also lets all-clear notifications through Do Not Disturb. To silence them, go to **Settings → Apps → ntfy → Notifications → Notification categories**, tap **Default priority** (its name, not its switch) and choose **Silent**. If **Notification categories** is missing, go to **Settings → Notifications → Advanced settings** and turn on **Manage notification categories for each app**.
- **ntfy on other Android phones**: in the phone's **Settings**, go to **Apps → ntfy → Notifications**, tap **Max priority** (its name, not its switch) and turn on **Override Do Not Disturb**.
- **ntfy on iPhone**: ntfy cannot sound when an iPhone is muted or on Do Not Disturb. Use Pushover or the Home Assistant app instead.

To check the setup, click **Send in 30 seconds**, then lock your phone and turn on Do Not Disturb. If the phone sounds, alerts will wake you.

## Sensor alerts

Use a Sensor alert to watch any sensor, such as CO2, a water tank level or a soil probe. Click **Add sensor alert**, then choose the sensor, **At or above** or **At or below**, and a value in the sensor's own unit.

Use the switch to turn a Sensor alert off without deleting it. Editing a Sensor alert ends its active alert without a notification. A new alert starts if the reading is still at or past the value. Sensor alerts are part of backups, and deleting a sensor deletes its alerts.

## Recent alerts

Lists the last 50 alerts, active ones first, with how each was delivered: **Sending**, **Sent**, **Sending failed**, **Not sent: notifications are off** or **Not sent: notifications are paused**.

Each alert shows the reading when it started, the highest or lowest reading while it was active, and the limit. If the limit changed during the alert, for example between lights on and lights off, each limit is listed. When alerts come from more than one zone or of more than one type, use **Zone** and **Type** to filter the list.

Click an alert to see its readings on the Sensors page, with a green line at the moment the alert started. The time the alert was active is shaded on its chart, and a dashed line shows the limit. The shading starts after 15 minutes at the limit and ends after 30 minutes back in range, so it begins and ends later than the points where the readings cross the limit.

Click **Clear** to move an ended alert to the **Cleared** tab, or **Clear All Ended** to move every ended alert shown. Click **Restore** to move one back. Active alerts cannot be cleared. Ended alerts are kept for 90 days, cleared or not. The list is not part of backups.

## Example scenarios

**Low VPD in Flower.** A zone in Flower, lights on, has a VPD Low limit of 1.0 kPa. The dehumidifier stops and VPD falls to 0.95 kPa. If every reading for 15 consecutive minutes stays at or below 1.0 kPa, a **VPD low** alert notification is sent. If the problem continues, reminders are sent every 6 hours. Once every reading stays above 1.0 kPa for 30 minutes, one all-clear is sent.

**High temperature while VPD looks normal.** Warm, humid air can keep VPD in range while the temperature is too high: 92 °F at 75% humidity is a VPD of about 1.3 kPa. With a High limit of 90 °F, a **Temperature high** alert notification is sent after 15 consecutive minutes at 90 °F or above, regardless of the VPD reading.

**CO2 running high.** A CO2 Sensor alert is set to **At or above** 1,500 ppm. A brief spike does not count, because one reading below 1,500 ppm resets the 15-minute count. After 15 consecutive minutes at 1,500 ppm or above, an alert notification is sent. Once every reading stays below 1,500 ppm for 30 minutes, one all-clear is sent.

**Light controller clock an hour off.** A zone's schedule says on at 6:00 AM and off at 6:00 PM, but the light controller's clock was not changed for daylight saving time, so the lights switch at 5:00 AM and 5:00 PM. At first you receive **Lights did not turn on or off** alert notifications. After three days, BudMinder sends one **Lights are off schedule** alert notification instead, listing the times the lights actually switched. To correct this, check the clock on the light controller or the time zone in **Settings → General**.
