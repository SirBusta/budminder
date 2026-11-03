BudMinder sends you an alert notification when a zone's climate, lights or sensors need attention. Manage alert notifications in **Settings → Alerts**.

> **Beta.** Alerts are still being tested. Do not rely on them as your only safety check. Feedback is appreciated: [budminder.app@gmail.com](mailto:budminder.app@gmail.com?subject=BudMinder%20alerts%20feedback).

## What sends an alert notification

| Alert | Sent when |
|---|---|
| **Climate (VPD, Temp & Humidity)** | Climate reading reaches a Low or High limit for 15 consecutive minutes |
| **Lights did not turn on or off** | Lights turn on or off outside the scheduled time for 15 consecutive minutes |
| **Lights are off schedule** | Lights switch about 1 hour before or after the schedule on each of the last 3 days |
| **Sensor alerts** | Sensor reaches a user defined value for 15 consecutive minutes |
| **Sensors offline** | No readings received for 15 minutes from a climate sensor, a light sensor or a sensor with a Sensor alert turned on |

Set limits in [Climate targets and limits](/manual/grow-setup#climate-targets-and-limits), in **Settings → Grow setup**. Temperature and humidity start with wide limits for each stage. VPD starts with Targets and no limits, so BudMinder sends no VPD alert notification until you set a limit.

Default Targets and limits are guidelines only. Choose values suited to your environment. If you receive too many alert notifications, widen your limits.

BudMinder checks climate and lights only in zones with plants, and only with the sensors chosen in **Edit Zone**. A sensor that never sent a reading does not count as offline.

## How often alert notifications are sent

| When | What happens |
|---|---|
| **First 15 minutes** | Nothing is sent. One reading back in range starts the 15 minutes over, so brief spikes are ignored. |
| **At 15 minutes** | The first notification is sent. |
| **Every 6 hours** | A reminder is sent until the problem ends. Every active alert is included in the same email. |
| **After 30 minutes back in range** | One final all-clear is sent. If a zone has several climate alerts, you receive one all-clear when the last one ends. |

**Lights are off schedule** is sent once, with no reminder or all-clear. It replaces **Lights did not turn on or off** while the lights stay about 1 hour off schedule.

- **Reaching a value counts.** A High limit of 1.50 kPa is reached at exactly 1.50.
- **BudMinder waits after lights turn on or off.** It ignores climate readings for the first 10 minutes, so a climate alert can start no sooner than 25 minutes after the change.
- **A sensor that stops sending readings** does not start or end a climate alert. After 15 minutes, BudMinder sends a **Sensors offline** alert notification.
- **Alerts that start together** arrive in one email. If an email fails, BudMinder tries again every 5 minutes, and **Recent alerts** shows the mail server's error.
- **Alerts end without an email** if you remove the limit, if you remove the sensor in **Edit Zone**, or if the zone has no plants left.

## Zone status

Shows each zone's current readings, limits and alerts, and the status of its lights and sensors. The bar for each reading shows these areas:

- **Target zone** (green): near the Target. If a limit is set, this is within half the distance from the Target to the limit.
- **Caution zone** (amber): between the Target zone and a limit. It appears only when a row has both a Target and a limit.
- **No target or High/Low limit set** (gray): no Target or limit applies to a reading here.
- **Alert zone** (red): at or past a limit.

The green mark is the Target. The other mark is the current reading. Reload the page to see new readings.

## Notification methods

Alert notifications are sent by email. Other methods are planned for a future release.

Click **Email** to open its settings. Enter your **Mail server**, **Username**, **Password**, **From address** and **Send to** addresses, then click **Send test email**. If the test email arrives, turn on the switch on the **Email** row. Gmail and most other mail services require an app password, not your account password. The password is stored encrypted and is left out of backups.

Before you can turn on that switch, set a limit in **Climate targets and limits**, add a **Sensor alert**, or set up a **Light check** in **Edit Zone**. With email off, alerts still appear under **Recent alerts**. Emails use the language BudMinder was showing when you last changed the Email settings.

## Sensor alerts

Use a Sensor alert to watch any sensor, such as CO2, a water tank level or a soil probe. Click **Add sensor alert**, then choose the sensor, **At or above** or **At or below**, and a value in the sensor's own unit.

Use the switch to turn a Sensor alert off without deleting it. Editing a Sensor alert ends its active alert without an email. A new alert starts if the reading is still at or past the value. Sensor alerts are part of backups, and deleting a sensor deletes its alerts.

## Recent alerts

Lists the last 50 alerts, active ones first, with how each was delivered: **Sending**, **Emailed**, **Email failed** or **Not sent: notifications are off**.

Click **Clear** to move an ended alert to the **Cleared** tab, or **Clear All Ended** to move every ended alert. Click **Restore** to move one back. Active alerts cannot be cleared. Ended alerts are kept for 90 days, cleared or not. The list is not part of backups.

## Worked examples

**Low VPD in Flower.** A zone in Flower, lights on, has a VPD Low limit of 1.0 kPa. The dehumidifier stops and VPD falls to 0.95 kPa. If every reading for 15 minutes stays at or below 1.0, a **VPD low** alert notification is sent. If the problem continues, reminders are sent every 6 hours. Once no reading has reached 1.0 for 30 minutes, one all-clear is sent.

**High temperature while VPD looks normal.** Warm, humid air can keep VPD in range while the temperature is too high: 92 °F at 75% humidity is a VPD of about 1.3 kPa. With a High limit of 90 °F, a **Temperature high** alert notification is sent after 15 minutes at 90 °F or above, whatever the VPD reads.

**CO2 running high.** A Sensor alert is set to at or above 1,500 ppm. A short spike does not count, because one lower reading starts the 15 minutes over. After 15 minutes at 1,500 ppm or more, the alert notification is sent. After 30 minutes under 1,500 ppm, one all-clear is sent.

**Lights an hour off.** A zone's schedule says on at 6:00 AM and off at 6:00 PM, but the controller's clock was not moved for daylight saving, so the lights switch at 5:00 AM and 5:00 PM. At first you receive **Lights did not turn on or off** alert notifications. After three days, BudMinder replaces them with one **Lights are off schedule** alert notification that lists the times the sensor saw. To correct this, check the clock on the light controller, or the time zone in **Settings → General**.
