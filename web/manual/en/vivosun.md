> **Beta.** Vivosun support has been tested only with the VCure C80 so far. Other Vivosun devices may be missing or show readings under the wrong names. See [Help add your device](#help-add-your-device).

BudMinder reads the sensors on your Vivosun devices through your Vivosun app account. It only reads: it never changes a device's settings. Readings come from Vivosun's servers, so BudMinder needs an internet connection, and each reading arrives about a minute after the device sends it.

## Connecting

1. Go to **Settings → Integrations → Vivosun** and click **Sign in**.
1. Enter the email and password of your Vivosun app account. Check **Stay signed in** (recommended; see below) and click **Sign in**. Signing in turns on **Read Vivosun sensors**.
1. Go to [Manage sensors](/manual/manage-sensors), click **Scan Vivosun**, choose a tent for each reading and click **Add**.

BudMinder records readings only for devices with at least one sensor added. It checks your account for new devices every 10 minutes.

## Stay signed in

Vivosun can sign BudMinder out. Without **Stay signed in**, readings stop until you sign in again, and the Today page shows a **Sign in again** warning.

With **Stay signed in**, BudMinder keeps your Vivosun password, encrypted, and signs in again by itself, at most once an hour. The encryption key is kept in a separate file in BudMinder's data folder, not in the database or in backups. Click **Forget saved password** to remove the saved password. Use a password you don't use anywhere else.

## VCure C80 readings

A scan lists these VCure readings, each with a short note saying what it measures:

- **Inside:** temperature, humidity and VPD of the air inside the VCure, as shown on its screen.
- **Room:** the same readings for the air in the room, outside the VCure. These start with **Home** off, so they do not show in the summary on the Today page.
- **Electronics and cooling coil temperatures:** the VCure's circuit board and its cooling coil, not the air. These also start with **Home** off.
- **Equipment:** whether the compressor is **On** or **Off**, and the level of the circulation fan, cooling coil fan, humidifier and heater (**Off** to 100%). These are listed only when you check **Show equipment**, and start with **Home** off.

The Wi-Fi signal and the VCure's settings are not listed.

## Other Vivosun devices

Other devices have not been tested yet. A scan lists these readings when a device sends them:

- **Air:** temperature, humidity and VPD from built-in sensors, probes and controllers, including the air under the VSeed's humidity dome. Readings of the air outside a device start with **Home** off.
- **CO₂, root temperature, water temperature and water level.** Water readings start with **Home** off.
- **Equipment:** the level of lights, fans, USB ports and outlets (**Off** to 100%), and each light's spectrum (color) setting. These are listed only when you check **Show equipment**, and start with **Home** off.

Equipment you add is charted below the climate charts on the [Environment](/manual/environment) page.

If two devices have the same name, BudMinder adds a number to the second device's readings (for example "VCure C80 temp (2)") so you can tell them apart.

## If readings stop

Vivosun doesn't report whether a VCure is online. When a VCure stops sending readings, the Vivosun card on [Manage sensors](/manual/manage-sensors) shows **No new readings**. If those sensors are used for alerts, a **Sensors offline** alert starts after 15 minutes with no readings (see [Alerts](/manual/alerts)).

If the card shows **Sign-in expired**, go to **Settings → Integrations → Vivosun** and click **Sign in again**.

## Help add your device

1. Go to [Manage sensors](/manual/manage-sensors). At the bottom of the page, under **Troubleshooting**, click **View raw data** next to Vivosun.
1. Click **Download**. The file holds what Vivosun sends about your devices, including the last hour of readings. Your email address and other account details are removed.
1. Check the file, then email it to [budminder.app@gmail.com](mailto:budminder.app@gmail.com?subject=BudMinder%20Vivosun%20data) with your device's model.

BudMinder's Vivosun support uses the request format worked out by the open-source [homeassistant-vivosun-growhub](https://github.com/lientry/homeassistant-vivosun-growhub) project (MIT License).
