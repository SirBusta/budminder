The VPD chart shows the leaf VPD (vapor pressure deficit) for every combination of air temperature and humidity, colored by how suitable it is for each growth stage. Click **Tools** in the top menu to open it.

- **Zone.** Choose a zone to see its current reading on the chart. The chart switches to the zone's growth stage and outlines the cell nearest its current temperature and humidity. The reading comes from the climate sensors chosen in the zone's **Edit Zone** window. Choose **No zone** to view the chart without a reading.
- **Stage.** Choose Seedling, Veg or Flower to see the colors for that stage. A tent icon marks the zone's current stage. Every Flower phase uses the Flower ranges.
- **Leaf temperature offset.** Leaf temperature minus air temperature. It starts from the zone's offset in **Edit Zone**, or −2 °F (−1.1 °C) if the zone has none. Changing it here affects only this chart.
- **Chart.** Rows are air temperature and columns are relative humidity. Each cell shows the leaf VPD in kPa, rounded to one decimal place, and is colored by that value.

## Color ranges

The colors follow commonly published leaf VPD ranges, in kPa, and cannot be changed:

| Stage | Ideal | Good | Slightly low or high |
|---|---|---|---|
| Seedling | 0.6–0.8 | 0.4–0.5, 0.9–1.0 | 0.2–0.3, 1.1–1.4 |
| Veg | 0.9–1.1 | 0.7–0.8, 1.2–1.3 | 0.5–0.6, 1.4–1.7 |
| Flower | 1.2–1.5 | 1.0–1.1, 1.6–1.7 | 0.7–0.9, 1.8–2.3 |

Values outside these ranges are shown as too low or too high. The VPD Targets in [Climate targets and limits](/manual/grow-setup#climate-targets-and-limits) do not change these colors. Alert notifications follow the Low and High limits set there, not the chart.

## Why the leaf offset matters

Plants respond to VPD at the leaf, not in the air. Leaves are usually a little cooler than the air around them, and a cooler leaf means a lower VPD. A chart based on air temperature alone shows a higher VPD than your plants actually experience.

To find your offset, measure leaf temperature with an infrared thermometer and subtract the air temperature. A leaf that is 2 °F (1.1 °C) cooler than the air gives an offset of −2 °F (−1.1 °C), the most common starting value. Under some lights, such as HPS, leaves can be warmer than the air, which gives a positive offset.

Other pages can show a slightly different VPD for a zone that uses a VPD sensor, such as an AC Infinity controller. The sensor uses its own leaf temperature offset, set on the device or in its app.
