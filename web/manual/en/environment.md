The Environment page charts every sensor in a tent on a unified timeline. Click **Sensors** in the top menu to open it, or click **Charts** on any tent panel on the Today dashboard.

![The Environment page: the tent buttons, the readings, the time range, a marked moment and the first chart](fig:environment)

- {1} **Tents.** Click any tent zone tab to display its active sensors.
- {2} **Current readings:** displays live metrics for each sensor in dedicated summary cards. Click any card to show or hide its corresponding chart. If a sensor stops reporting, its card displays the timestamp of the last recorded reading.
- {3} **Time navigation:** use the left and right arrows to step forward or backward through time by the length of your selected range.
- {4} **Time range:** choose preset intervals (1 hour, 6 hours, 24 hours or 7 days) or select **Custom** to set a specific date and time window.
- {5} **Synchronized inspection line:** click anywhere on a chart to drop a vertical marker across every graph, displaying exact sensor readings at that specific moment. Drag the line across the timeline to inspect changes throughout the day. Click the × icon to clear the marker.
- {6} **Sensor analytics:** each chart displays the current reading (or the value at your inspection marker), alongside the minimum, maximum and average values calculated over the active time range.
- {7} **Manage sensors:** opens the sensor administration dashboard to pair, rename or assign devices. See [Manage sensors](/manual/manage-sensors).

## Devices

For tents connected to an AC Infinity controller, the lower section of the page charts active device ports (such as exhaust fans, circulation fans, lights and humidifiers), plotting their output power levels over the same time range. A reading of **Off** indicates the device was idle.

## No sensors showing?

If a tent displays no charts, its sensors have not yet been assigned to that zone or have been set to hidden. You can adjust zone assignments and visibility settings under [Manage sensors](/manual/manage-sensors).

## Exporting readings

Click **Export readings**, next to **Manage sensors**, to save sensor readings as a spreadsheet file. The link appears only when you are signed in.

- **Time span:** choose **Last 7 days**, **Last 30 days** or **All dates**, or enter your own **From** and **To** times. Times use the time zone set in Settings > General.
- **Sensors:** sensors are grouped by tent, and the tent you are viewing starts selected. **All** and **None** select or clear a whole tent. Turn on **Show hidden sensors** to include sensors hidden from the Environment page.
- **Interval:** **Every reading** exports each reading as recorded. **15 minutes**, **30 minutes**, **1 hour** and **1 day** export the average, lowest and highest reading for each period. Periods start on the clock (for example 2:00, 2:15, 2:30).
- **Layout:** **One row per sensor and time** puts the time, zone, sensor and value on each row. **One column per sensor** gives each sensor its own column, which is easiest to chart.
- **File type:** **CSV** opens in any spreadsheet program. **Excel** creates an .xlsx file of up to 1,048,576 rows. For a larger export, **Download** stays off until you choose CSV, a longer interval or a shorter time span.

The **Preview** shows the first rows of the file and its approximate size. Temperatures use the measurement units set in Settings > General.

> **Note:** If **Sensor data retention (days)** in Settings > Backup & data is above 0, BudMinder deletes older readings but keeps hourly averages. **Every reading**, **15 minutes** and **30 minutes** then reach back only as far as the readings still kept. **1 hour** and **1 day** reach back to the first reading.
