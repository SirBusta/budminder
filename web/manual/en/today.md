Today is your main dashboard. It shows pending tasks, followed by each tent with its live climate readings, camera feed and active plants. Click **Home** in the top menu to return here at any time.

## Needs attention

![The top of the Today page: the Log activity and Add photos buttons, then the Needs attention list](fig:today-attention)

- {1} **Log activity** opens the logging window to record watering, feeding, plant training, measurements or notes for any plant.
- {2} **Add photos** lets you upload new photos directly to a plant.
- {3} **Each line is something to check or do.** By default, the list shows:
  - plants not watered in 3 days or more (red from 6 days)
  - plants not fed in 10 days or more (red from 18 days)
  - plants without a new photo in 14 days
  - sensors that have stopped reporting, and integrations (AC Infinity, Vivosun, Ecowitt or Home Assistant) that stop responding or need you to sign in again
  - strains with seeds older than 2 years
  - [repeating tasks](#repeating-tasks) that are due
- {4} **Action buttons let you resolve items quickly.** **Watered it** or **Fed it** opens the logging window with the plant, the activity and today's date already filled in. Other lines provide direct shortcuts such as **Add photo**, **Check sensors**, **Sign in again** or **Open Seed Vault**.

When everything is fine, the list says **Nothing needs attention right now**.

### Hiding a line

Click **×** at the end of a line:

- **Remind me tomorrow** hides the line until the next day.
- **Hide until…** hides it until the item changes: for example, until the plant is watered or the sensor reports again. If the problem returns later, so does the line.

Hidden lines are listed under **Hidden**, below the list. **Show again** brings a line back. Right after you hide a line, **Undo** at the bottom of the page reverses it.

### Needs attention settings

Click **Settings** next to the Needs attention heading to choose what the list includes.

- The **Reminders** tab turns each type of reminder on or off and sets its days. Watering and Feeding have two numbers: **Due** (amber) and **Overdue** (red). These numbers also color the watering and feeding indicators in each tent and on the Plants page, even when the reminder is off. Integration problems are always on.
- The **Repeating tasks** tab is where you add and edit tasks. See [Repeating tasks](#repeating-tasks).

Changes take effect when you click **Save**.

## Repeating tasks

Repeating tasks remind you of work you do on a schedule, such as refilling a reservoir or emptying a dehumidifier.

1. Click **Settings** next to the Needs attention heading, then open the **Repeating tasks** tab.
2. Click **Add task**, then choose a suggestion or type a name, and set how often the task repeats.
3. If you have more than one tent, choose which tents the task is for. Each tent keeps its own count.
4. Click **Save task**, then **Save**.

A task appears in the list on the day it is due and turns red once it is late. Click **Done** to restart the count from today. In the **×** menu, **Skip this time** also restarts the count, and **Remind me tomorrow** hides the task until the next day. Tasks are not recorded in plant journals. To keep a record, log an activity.

**Burp curing jars** is a built-in task for plants in the Curing stage. It is off by default: turn it on in the **Repeating tasks** tab. When it is on, it reminds you every day for the first 14 days of curing, then every 7 days. You can change both numbers. One line covers every plant that is due, and **Burped** marks them all done.

## Tents

Each tent zone has its own panel showing its live environment, camera feed and plants.

![One tent on the Today page: its readings, the camera picture and two plant cards](fig:today-tent)

- {1} **Readings** from the tent's sensors, such as temperature, humidity, VPD and CO2, plus whether the lights are on or off if controlled by an AC Infinity port.
- {2} **Charts** opens the Environment page for this tent to view detailed sensor history.
- {3} **The camera**, if the tent has one: the latest snapshot and how old it is. Click **Live** to watch it.
- {4} **When it was last watered and fed.** These indicators turn amber, then red, following the same time thresholds as the Needs attention list.
- {5} **The harvest estimate**, in weeks, for plants in Flower, calculated from the strain's flowering time. See [Plant page](/manual/plant-page#growth-stages).
- {6} **Water**, **Feed** and **Log** open the activity logging window for this plant. The volume given last time is shown, making quick repeats easy.

Click a plant's name to open its page, or the camera button on its photo to add photos. Plants that aren't in a tent are listed under **No tent**.

## Recent activity

At the bottom of the page, the last few logged events appear for all plants. Open notebook displays a combined activity log across all of your zones.
