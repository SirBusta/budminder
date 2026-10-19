Manage sensors is where you find new sensors, name them, put each in the right tent and choose where its readings show. Open it from **Manage sensors** on the Environment page.

## Connections

At the top, a card for each source (AC Infinity, Ecowitt and Home Assistant) shows whether it's connected, how many sensors it has and when it last sent readings. If something is wrong, the card says what: for example **Sign-in expired**, **Not responding** or **No new readings**. **Account & polling** opens its settings.

## Finding new sensors

Click **Scan AC Infinity**, **Scan Ecowitt** or **Scan Home Assistant**. For Ecowitt, enter the gateway's address on your network first.

BudMinder lists the new sensors it found, with their current readings. For each one, choose a tent (or **New tent…** to make one) and click **Add**, or **Add all**. **Ignore** skips a sensor you don't want.

Home Assistant knows about many things that aren't grow sensors, so the scan shows the useful ones first. Type in **Filter found sensors** to narrow the list, or turn on **Show all** to see the rest, such as batteries and power.

Sensors that send readings through the API appear here on their own. See [Your own sensors](/manual/your-own-sensors).

## Your sensors

![Manage sensors: the filters, a tent's header and its sensors with their chips and Edit buttons](fig:manage-sensors)

- {1} **Filter sensors** finds sensors by name as you type.
- {2} **All sources** narrows the list to AC Infinity, Ecowitt, Home Assistant or your own. Tick **Show hidden** to include hidden sensors.
- {3} **Each tent** lists its sensors, with **View charts** to open its Environment page.
- {4} **Where it shows**: **Environment** (the tent's charts), **Home** (the tent on the Today page), the plants it's linked to, or **Hidden**.
- {5} **Edit** opens the sensor's settings.

## Editing a sensor

- **Name**: what BudMinder calls it everywhere.
- **Tent / zone**: which tent it belongs to.
- **Unit**: the unit the sensor reports in. Changing it doesn't convert readings already recorded.
- **Shows on**: the Sensors page, the Home page, both, or **Hide everywhere**. A hidden sensor keeps recording.
- **Linked plants**: plants in the tent that this reading belongs to.

**Delete sensor** removes it and all its readings, after asking.

> **Tip:** At the bottom of the page, **Troubleshooting: view the raw AC Infinity data** shows exactly what AC Infinity sends, with an option to hide personal details before sharing it.
