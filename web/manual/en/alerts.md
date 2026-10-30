**Settings → Alerts** lets BudMinder email you when a zone's climate or lights need attention. After each sensor poll, it compares each zone's chosen climate sensors with its [climate targets](/manual/grow-setup#climate-targets) and light schedule.

## Email

Enter your mail server, sign-in, From address and recipients, turn on **Send alert emails**, then click **Send test email**. Gmail and most other mail services require an app password, not your account password. The password is stored encrypted and is left out of backups.

## Climate alerts

**Mode** decides what happens when an alert starts:

- **Off**: nothing is checked.
- **Dry run** (the default): alerts are listed under **Recent alerts**, but no email is sent. Use it to test your limits in a real tent before turning on email.
- **Send email**: alerts are listed and emailed. If the Email card is off or incomplete, alerts are listed as **Not sent: email is off**.

| Alert | Starts when | Repeats while active | Back-to-normal email |
|---|---|---|---|
| **Safety limit** (temperature, humidity) | 2 readings in a row reach the limit | Every 30 minutes | Yes |
| **Low or High** (VPD; temperature and humidity with Targets by stage) | Most readings in the last 10 minutes reach it | Every 2 hours | Yes |
| **Drifting** | Readings stay amber on one side of Target for 2 hours | No, one notice | No |
| **Lights did not turn on or off** | The light sensor disagrees with the schedule for 15 minutes | Every 2 hours | Yes |
| **Lights are off schedule** | Lights switch about 1 hour before or after the schedule on each of the last 3 days | No, one notice | No |

These times, except the two light checks, can be changed on the card. A few rules keep alerts quiet and accurate:

- **Reaching a value counts.** A High of 1.50 kPa is reached at exactly 1.50.
- **Short swings are ignored.** Low and High use the middle reading of the window, so an open door doesn't send an alert.
- **Pause after a light switch.** Low, High and drifting checks wait 10 minutes after lights turn on or off, and readings from before then don't count, so a real problem alerts about 13 minutes after the switch. Safety limits keep running.
- **Back to normal needs a small margin:** 0.5 °C (about 1 °F), 2% humidity or 0.05 kPa inside the limit.
- **A sensor that stops reporting** neither sends an alert nor ends one.
- **Several alerts at once** arrive in one email, with the most serious in the subject. A failed email is tried again every 5 minutes, and the list shows the mail server's answer.
- **Alerts end without an email** when they no longer apply: the sensor was unassigned, the zone has no plants left, or alerts were turned off.
- **Emails use the language** that was in use when the Climate alerts card was last saved.

Zones without plants, and sensors not chosen in **Edit zone**, aren't checked.

## Recent alerts

Lists the last 50 alerts, active ones first, with how each was delivered: **Dry run**, **Emailed**, **Email failed** or **Not sent: email is off**. Ended alerts are kept for 90 days. The list isn't part of backups.

## Worked examples

**Low VPD in Flower.** A zone in Flower, lights on, has VPD Low 1.0 · Target 1.25 · High 1.5. The dehumidifier stops, and VPD falls to 0.95 kPa. About 5 minutes later most of the last 10 minutes' readings are at or below 1.0, so the **VPD low** alert starts. It repeats every 2 hours. Once the middle reading climbs above 1.05 kPa, you get one back-to-normal email.

**High temperature while VPD looks normal.** Warm, humid air can keep VPD inside its range while the temperature is too high: 92 °F at 75% humidity is a VPD of about 1.3 kPa, inside the range above, yet the plants are hot. With a high safety limit of 90 °F, two readings in a row at 90 °F or above start a **Temperature high** alert, whatever the VPD says. It repeats every 30 minutes until two readings in a row are about 1 °F under the limit.

**Lights an hour off.** A zone's schedule says on at 6:00 AM and off at 6:00 PM, but the controller's clock wasn't moved for daylight saving, so the lights switch at 5:00 AM and 5:00 PM. For the first days you get **Lights did not turn on or off** alerts. After three days BudMinder sees the steady offset, replaces them with one **Lights are off schedule** notice and tells you what it saw. Check the clock on the light controller, or the time zone in **Settings → General**.
