**Settings → Grow setup** manages the foundational lists and parameters powering BudMinder: tent zones, climate targets and limits, activity types, environmental metrics and evaluation fields.

## Zones

Zones represent your physical growing spaces, such as tents, rooms or outdoor gardens. Each plant, climate sensor and camera feed maps directly to a zone. The management list displays the number of active plants and sensors assigned to each space. Click **Add Zone** to create a new location, or click an existing zone to rename or delete it.

> **Note:** A zone can only be deleted once it's empty. If it still has plants, sensors or cameras, BudMinder says how many and keeps the zone. Move them to another zone (or delete them) first.
>
> **Tip:** You can also create a new zone on the fly while setting up a plant (**Add New Zone**) or adding a sensor (**New tent…**).

The **Light schedule** column shows the zone's lights-on time and hours for its current stage, and **Now** shows whether the lights are on or off and which stage the zone follows.

### Edit Zone

- **Light schedule**: choose **On a schedule** and enter the lights-on and lights-off times for Seedling, Veg and Flower, as set on your lights or controller. BudMinder does not control grow lights; the schedule only decides whether the lights-on or lights-off climate targets apply. Times use the time zone in **Settings → General**, and the window shows BudMinder's time now so you can compare it with your controller. Choose **No schedule** for outdoor zones: the lights-on targets then apply all the time. Drying always uses the lights-off targets.
- **Light check** (optional): pick a light port, lux sensor or smart plug in the zone and the reading above which the lights count as on (0 for an on/off port). The window shows whether the sensor agrees with the schedule right now. If they disagree for 15 minutes, or the lights switch about an hour off the schedule three days running, BudMinder sends an alert notification. See [Alerts](/manual/alerts).
- **Plant stage**: when every plant in the zone is in the same stage, the zone follows it. When stages differ, choose which plant to follow; by default it's the plant furthest along. Drying and Curing plants are skipped while others are still growing, and Germinating and Planted count as Seedling. If the chosen plant leaves the zone, the zone follows the plant furthest along again. A zone without plants shows no colors.
- **Climate sensors that can trigger alerts**: the temperature and humidity sensors that measure the zone's climate, and where its VPD comes from: **From a sensor** (use the sensor's own VPD when it has one, so it matches its app) or **Calculated** from the temperature and humidity, with a leaf temperature offset (leaf temperature minus air temperature, usually −2 to −4 °F).

## Climate targets and limits

Set the Target and the Low and High limits for each growth stage. Choose **VPD**, **Temperature** or **Humidity** at the top of the card. Changes are saved as you type.

- **Limits** are Low and High. A reading that **reaches** a limit shows **red**, and an alert notification is sent after 15 consecutive minutes. See [Alerts](/manual/alerts).
- **Defaults:** Temperature and humidity start with wide limits for each stage and no Targets. VPD starts with Targets and no limits, so BudMinder sends no VPD alert notification until you set a limit. VPD does not apply to Drying. Default Targets and limits are guidelines only. Choose values suited to your environment.
- **Colors:** Readings near the Target show **green**, and readings between the Target and a limit show **amber**. "Near" means within half the distance from the Target to the limit. If a Target has no limit above or below it, "near" on that side means within 0.2 kPa, 5 °F or 7% humidity. Readings further away show no color. With no Target, only readings at a limit show a color.
- **Valid values:** A Target must be between its limits, and a Low limit must be below its High limit. An invalid value is marked red with a message under the row, and BudMinder keeps using the last valid value.
- **Targets and limits by stage:** Turn this off to use one pair of limits for every stage, lights on and off.
- **Separate lights-off values** adds a second set of values used while a zone's light schedule says the lights are off.
- **Split Flower into phases** divides Flower into **Stretch**, **Full bloom** and **Ripening**. Set the last week of Stretch and of Full bloom in the boxes under their names, and the next phase starts the week after. VPD, Temperature and Humidity share the same weeks. A plant's Flower week counts from the day it entered Flower.
- Each stage row shows a tag for each zone in that stage now. The bar under the rows shows where one zone's current reading falls.
- **Climate sensors by zone** lists the sensors BudMinder checks against these Targets and limits. Set them in each zone's **Edit Zone** window.
- **Alert notifications** at the bottom of the card shows whether notifications are on. See [Alerts](/manual/alerts).

Colors appear on the Home and Environment pages for each zone's chosen climate sensors, while the zone has plants and the sensor is sending readings. Temperatures follow the measurement units in **Settings → General**.

## Activities

Activities are the actions you log in a plant's notebook, such as Water, Feed, Training, Defoliate or Height. Click **Add Activity** to define a new action, or click an existing entry to edit it:

- customize its **name**, **icon** and **color**, which appear throughout the daily notebook timeline;
- **Counts as**: check **Watering** or **Feeding** for actions that should reset the "last watered" or "last fed" warning counters on the Today dashboard and Plants list;
- **measurements**: configure numeric metrics prompted when logging this action. Fields marked with an asterisk (*) are mandatory, while unmarked metrics remain optional.

## Metrics

Metrics represent numeric data points you track in the journal, such as Height, Water amount, pH, EC or Runoff EC, each paired with its specific **Unit**. Click **Add Metric** to create a custom tracking field. Attach metrics to specific activities above so they appear automatically when logging that action.

## Breeders

Manage your master breeder directory. See [Breeders](/manual/breeders).

## Smoke reports

Configure rating category weights and toggle visible review fields. See [Scores and weights](/manual/scores-and-weights#score-weights).
