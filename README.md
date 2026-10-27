<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="web/static/img/brand/budminder-wordmark-dark.svg">
  <img src="web/static/img/brand/budminder-wordmark.svg" alt="BudMinder" width="340">
</picture>

**A self-hosted cannabis grow journal.** Daily notes and photos for every plant, live sensors from AC Infinity, Ecowitt or Home Assistant, and your seed vault, all on your own hardware.

Runs in **Docker** (preferred), for example on Unraid, a Synology NAS or a Raspberry Pi, or as a download for **Windows**, **macOS** and **Linux**.

<a href="#install"><img src="https://img.shields.io/badge/Docker-preferred-16a34a?logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCA2NDAgNTM0Ij48cGF0aCBmaWxsPSIjZmZmIiBkPSJNMzUwIDI0OEgyODRIMzUwSDI4NFYxODlIMzUwVjI0OFpNMzUwIDQ0SDI4NEgzNTBIMjg0VjEwNUgzNTBWNDRaTTQyOCAxODlIMzYySDQyOEgzNjJWMjQ4SDQyOFYxODlaTTI3MiAxMTdIMjA2SDI3MkgyMDZWMTc3SDI3MlYxMTdaTTM1MCAxMTdIMjg0SDM1MEgyODRWMTc3SDM1MFYxMTdaTTYyNyAyMTdRNjE1IDIwOSA1OTQgMjA3UTU3NCAyMDUgNTU0IDIwOFE1NDkgMTczIDUxMyAxNDVMNDk5IDEzNUw0ODkgMTQ5UTQ3NSAxNzEgNDczIDIwMFE0NzEgMjI5IDQ4NiAyNTNRNDcyIDI2MiA0MzcgMjY0SDJRLTQgMzAzIDcgMzQ3UTE4IDM5MSA0NiA0MjZRMTAyIDQ5MiAyMTIgNDkyUTMzMSA0OTIgNDE1IDQzOVE0OTkgMzg3IDU0MCAyODhRNTU4IDI5MCA1ODUgMjgyUTYxMiAyNzUgNjMxIDI0M1E2MzQgMjM4IDYzOSAyMjhRNjM5IDIyNyA2NDAgMjI2TDYyNyAyMTdaTTExNiAxODlINTBIMTE2SDUwVjI0OEgxMTZWMTg5Wk0xOTQgMTg5SDEyOEgxOTRIMTI4VjI0OEgxOTRWMTg5Wk0yNzIgMTg5SDIwNkgyNzJIMjA2VjI0OEgyNzJWMTg5Wk0xOTQgMTE3SDEyOEgxOTRIMTI4VjE3N0gxOTRWMTE3WiIvPjwvc3ZnPg%3D%3D&logoColor=white" alt="Docker: preferred"></a>
<a href="#windows-step-by-step"><img src="https://img.shields.io/badge/Windows-download-16a34a?logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCA0NDggNTM0Ij48cGF0aCBmaWxsPSIjZmZmIiBkPSJNMCAxMDYgMTg0IDgwIDAgMTA2IDE4NCA4MFYyNThIMFYxMDZaTTAgNDMwIDE4NCA0NTYgMCA0MzAgMTg0IDQ1NlYyODBIMFY0MzBaTTIwNCA0NTggNDQ4IDQ5MiAyMDQgNDU4IDQ0OCA0OTJWMjgwSDIwNFY0NThaTTIwNCA3OFYyNThWNzhWMjU4SDQ0OFY0NEwyMDQgNzhaIi8%2BPC9zdmc%2B&logoColor=white" alt="Windows: download"></a>
<a href="#macos-step-by-step"><img src="https://img.shields.io/badge/macOS-download%20%28beta%29-16a34a?logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAzODQgNTM0Ij48cGF0aCBmaWxsPSIjZmZmIiBkPSJNMzE5IDI4MVEzMTggMjI2IDM2OSAxOTZRMzQwIDE1NiAyODQgMTUxUTI2NyAxNTAgMjUwIDE1NVEyMzMgMTYwIDIyMCAxNjVRMjA0IDE3MSAxOTYgMTcyUTE4NSAxNzEgMTY4IDE2NFExNDQgMTU0IDExOSAxNTJRNzUgMTUzIDQxIDE4NlE2IDIxOSA0IDI4NlE0IDMyNSAxOCAzNjdRMjUgMzg2IDQxIDQxNlE1NyA0NDUgNzkgNDY4UTEwMSA0OTEgMTI2IDQ5MlExNDIgNDkxIDE1OCA0ODRRMTc2IDQ3NSAyMDEgNDc0UTIyNSA0NzUgMjQyIDQ4M1EyNTggNDkxIDI3OCA0OTJRMzAyIDQ5MCAzMjMgNDY5UTM0NCA0NDcgMzU5IDQyMFEzNzQgMzkyIDM4MCAzNzNRMzM1IDM0OCAzMjYgMzE3UTMxNyAyODcgMzE5IDI4MVEzMTkgMjgxIDMxOSAyODFaTTI2MiAxMTdRMjgxIDkyIDI4NSA3MlEyODggNTMgMjg2IDQ0UTI4NiA0NCAyODYgNDRRMjY4IDQ1IDI0OSA1NVEyMzEgNjUgMjE4IDc5UTE5MSAxMDkgMTkzIDE1MVEyMzIgMTUyIDI2MiAxMTdaIi8%2BPC9zdmc%2B&logoColor=white" alt="macOS: download (beta)"></a>
<a href="#linux-step-by-step"><img src="https://img.shields.io/badge/Linux-download-16a34a?logo=linux&logoColor=white" alt="Linux: download"></a>

[![Latest release](https://img.shields.io/github/v/release/SirBusta/budminder?color=16a34a&label=release)](https://github.com/SirBusta/budminder/releases)
[![Docker image](https://img.shields.io/badge/docker-ghcr.io%2Fsirbusta%2Fbudminder-16a34a?logo=docker&logoColor=white)](https://github.com/SirBusta/budminder/pkgs/container/budminder)
[![License: MIT](https://img.shields.io/badge/license-MIT-16a34a)](LICENSE)

**[Live demo](https://demo.budminder.app)** · [Website](https://budminder.app) · [Install](#install) · [Tour](#tour) · [Releases](https://github.com/SirBusta/budminder/releases)

</div>

<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/plant.webp" alt="A plant's page in BudMinder: the stage track from germination to harvest, a notebook with one entry per day, and the three photos taken on the selected day">

<p align="center"><sub>Every plant gets a stage track from seed to harvest and a notebook with one entry per day, shown next to that day's photos.</sub></p>

**Try it first:** the [live demo](https://demo.budminder.app) runs in your browser with six growing plants, a month of sensor history and smoke reports. Nothing to install, and your changes are private and reset after an hour of inactivity.

## Why BudMinder?

**Your grow, on your hardware.** BudMinder is a self-hosted cannabis grow journal designed to run entirely on your own machine or home server. No cloud accounts, no corporate tracking, and no treating your cultivation logs as a commodity. What you record stays on your own hardware.

**Free and open source.** BudMinder is free, with no subscriptions or paid tiers. Its code is public under the MIT license, so anyone can check exactly what it does with your data, suggest improvements, or build on it.

**Connect actions to results.** Each day's notes, watering and feeding sit right next to that day's photos. Scroll back through a grow and you can see how each change showed up in the plants: when you topped, when you flipped, when the frost came in.

**Close the loop from seed to smoke.** Write a smoke report for each harvest or dispensary pickup. Rate bud quality, aroma, flavor and effects. BudMinder auto-fills the dry and cure times so months later you can see which runs were worth repeating and whether an extended cure paid off.

**Instant strain lookups.** Search large public strain databases and add a strain in one click. That brings in its breeder, lineage, THC and CBD, effects, terpenes and flowering time. You only fill in what's yours: how many seeds you have and where you keep them.

**Use the hardware you already own.** BudMinder works with AC Infinity controllers and Ecowitt gateways, plus any sensor tracked by Home Assistant: Govee, SwitchBot, Inkbird, Zigbee, ESPHome and more. If Home Assistant sees it, BudMinder can plot it alongside your plant logs.

**Everything in one place.** Live sensor readings, tent cameras, harvest yields and a seed vault sit alongside your daily grow journal. BudMinder replaces the fragmented mess of vendor apps, spreadsheets and paper notes with a single app built around your grow.

**Easy to back up, simple to move.** Create a snapshot in one click, packaging your plants, notes, strains, smoke reports and photos into a single portable file. Restoring brings everything back or moves your installation to a new machine. You can also export your journal entries to CSV or Excel, with PDF export coming soon.

**Optional read-only guest view.** Want to show off your grow without compromising your records? Turn on guest view to let friends browse your plants, seed vault, smoke reports and photos without needing an account or being able to edit your data. Leave it off, and BudMinder stays completely private behind your login.

BudMinder began as a fork of the excellent [Isley](https://github.com/dwot/isley) by dwot, and builds on it with every page redesigned around a cleaner, more visual layout, plus new features like the daily photo notebook, Home Assistant sensors and multi-site strain lookups.

## Tour

### Plant pages

Every plant gets its own page and notebook.

**Visual stage tracking.** Follow each plant from germination through harvest. See the number of days spent in each stage and view projected harvest dates calculated from the strain's flowering window. Advance through the growth stages with a single click.

**Streamlined daily logs.** Each day's entry shows water amounts, training, height measurements and notes, right beside that day's photos.

**Flexible notebook views.** Filter by activity type, switch between a weekly and a list view, search your notes, or see all plant notebooks in an aggregate view.

**Logging, camera and export.** Log an activity for every plant in a tent in one go, watch the tent camera live, and export the notebook to CSV or Excel.

<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/plant-pm1-dark.webp" alt="Permanent Marker #1's page in dark mode: the stage track at Flower week 2 with an estimated harvest of Nov 30 to Dec 7, the notebook grouped by week with water amounts, heights and notes, and the two photos from Sep 28 shown beside the selected day"><br>
<sub>Shown in dark mode.</sub>

### Today

Open BudMinder and see what needs doing: plants you haven't watered or fed in a while, seeds getting old, and every tent's live temperature, humidity, VPD and CO₂ next to its camera and plants. Log water or feeding right from a plant's card.

<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/home.webp" alt="The Today page: a needs-attention list, then each tent with its live readings, camera snapshot and plant cards">

### Environment

Every sensor in a tent on one timeline, from the last hour to the last week or any range you pick. Click anywhere on a chart and a marker line shows what every sensor read at that moment; drag it to scrub through the day. Works with AC Infinity controllers, Ecowitt gateways and any sensor in Home Assistant. If you build your own sensors, a simple API lets you send in their readings.

<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/environment.webp" alt="The Environment page for a tent: current readings, then temperature, humidity and VPD charts for the last 24 hours with a marker line showing the readings at 3:25 AM">

### Seed Vault

Track the seeds you have on hand, how old they are and where you keep them, with a warning when a batch passes two years. Use one, restock, or start a grow straight from the list, and keep a wish list of strains you want.

<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/strains.webp" alt="The Seed Vault in list view, oldest seeds first, showing each strain's lineage, type, flowering time, seed count, seed age and grows">

### Strain pages

A complete archive for every cultivar you run. Track your full history with a strain, including past harvest yields, flowering times and photos from your grows, alongside its lineage, potency and cultivation traits. When adding a new strain, import it from public strain databases to fill in most of the profile automatically.

<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/strain-grape.webp" alt="The Grape OG strain page: four harvested plants with yields, the strain's family, seed vault and strain facts, and photos from the grows">

### Smoke reports

A complete evaluation suite for home harvests and dispensary pickups. Rate appearance, aroma, flavor and effects on a 1 to 10 scale to generate a custom weighted overall score. Pick traits, side effects and terpene profiles from built-in lists or enter your own, then flag whether the cultivar is worth growing or buying again. For dispensary purchases, log the store and price to automatically calculate the cost per gram. Attach photos by uploading a file, pasting an image URL, or selecting a shot directly from the plant's journal history.

<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/smoke-report.webp" alt="A smoke report for home-grown Grape OG: a photo of two dried buds, an overall score of 7.8 out of 10, THC, CBD and cure time, top effects and smells, bars for the bud quality, taste, smell and effects scores, and the grow details BudMinder filled in from the plant">

### Home Assistant

**No sensor left behind.** Connect BudMinder to Home Assistant to pull climate readings from any device you run, from Zigbee and ESPHome probes to Govee and SwitchBot units. A quick scan shows your grow-relevant sensors so you can add them straight to your tents, right alongside your AC Infinity and Ecowitt data. It operates completely read-only, leaving your smart home configuration safe and unchanged.

<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/ha-scan.webp" alt="Scan Home Assistant results: seven grow sensors such as tent temperature, humidity, CO2, VPD, light level and soil moisture, each with its current reading, a tent to put it in and an Add button, plus a filter box and a Show all switch for batteries and power">

### More

<table>
<tr>
<td width="50%" valign="top"><b>Plants</b><br>Every plant with its stage, progress to harvest, and when it was last watered and fed.<br><br><img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/plants.webp" alt="The Plants page listing six growing plants with stage, progress bar and days since watering and feeding"></td>
<td width="50%" valign="top"><b>Log activity</b><br>Water, feed, train or add a note for one plant or a whole tent at once.<br><br><img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/log-activity.webp" alt="The Log activity window with plants, date, activity types, water amount and a note"></td>
</tr>
<tr>
<td valign="top"><b>Manage sensors</b><br>Scan AC Infinity, Ecowitt and Home Assistant for sensors, name them, put each in a tent and choose where it shows.<br><br><img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/manage-sensors.webp" alt="The Manage sensors page listing a tent's sensors with their current readings"></td>
<td valign="top"><b>Settings</b><br>Grouped and searchable, and every change saves as you go.<br><br><img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/settings.webp" alt="The Settings page with timezone, units, language, theme, guest view and password"></td>
</tr>
<tr>
<td colspan="2" valign="top"><b>Help</b><br>Click <b>Help</b> in the top menu to learn about the page you're on. Every page is explained step by step, with numbered pictures that match the text, a search box, and light and dark versions.<br><br><img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/manual.webp" alt="BudMinder's Help: sections listed on the left, the Growth stages section of Plant page with a picture of the stage track marked with numbered green circles 1 to 5, the matching numbered list below it, and On this page on the right"></td>
</tr>
</table>

### On your phone

<p align="center">
<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/phone-home.webp" alt="The Today page on a phone" width="260">&nbsp;&nbsp;&nbsp;
<img src="https://raw.githubusercontent.com/SirBusta/budminder/main/docs/images/readme/phone-plant.webp" alt="A harvested plant's page on a phone in dark mode" width="260">
</p>

### And also

- A short guided tour for new users, with animated tips, which you can take again any time from Help
- One-click strain import from public strain databases, with THC and CBD ranges, effects, terpenes and lineage
- Hide the smoke report fields you never use, and choose how much each score counts
- Imperial or metric units for heights, yields and temperatures
- Everything is served from your own server: no outside fonts, scripts or trackers
- Journal export to CSV or Excel (PDF coming soon)
- Docker images for regular PCs and ARM (Raspberry Pi, newer Macs), plus ready-to-run programs for Windows, macOS and Linux
- Light and dark themes, in English, German, Spanish and French
- A tent camera: live view plus automatic snapshots
- Guest view for sharing, read-only
- SQLite or PostgreSQL

<sub>Screenshots use the same data set as the [live demo](https://demo.budminder.app). The sensor history is a month of real readings from an AC Infinity controller in a 3x3 tent.</sub>

## Install

There are two ways to run BudMinder:

- **In Docker (preferred):** on a home server or any computer with Docker, so BudMinder keeps running all the time. Follow the steps right below. You need [Docker](https://docs.docker.com/get-docker/) with Compose.
- **As a program on your computer:** download it for Windows, macOS or Linux and run it. See **Download for Windows, macOS or Linux** further down, with step-by-step guides for each.

**1. Save this as `docker-compose.yml`** (or use [`docker-compose.sqlite.yml`](docker-compose.sqlite.yml) from this repository):

```yaml
services:
  budminder:
    image: ghcr.io/sirbusta/budminder:latest
    ports:
      - "8080:8080"
    environment:
      - BUDMINDER_DB_DRIVER=sqlite
    volumes:
      - budminder-data:/app/data
      - budminder-uploads:/app/uploads
    restart: unless-stopped

volumes:
  budminder-data:
  budminder-uploads:
```

**2. Start it:**

```bash
docker compose up -d
```

**3. Open `http://localhost:8080`** (or your server's address).

> [!TIP]
> The first time you open BudMinder, it asks you to create a local account: choose your username and password. Do this right away, before anyone else on your network opens it.

To update later, run `docker compose pull` and then `docker compose up -d`. Your data stays in the two volumes.

<details>
<summary><b>Unraid</b></summary>

<br>

BudMinder is in **Community Applications**:

1. Open the **Apps** tab, search for **BudMinder**, and click **Install**.
2. Check the **WebUI** port (8091 is suggested; any free port works). The Data and Uploads folders are already filled in (`/mnt/user/appdata/budminder/…`).
3. Click **Apply**. When it's running, click the BudMinder icon on the **Docker** tab and choose **WebUI**, then create your local account.

**Without the Apps tab** (the Community Applications plugin not installed), set it up by hand:

1. In the **Docker** tab, click **Add Container**. Leave **Template** empty.
2. Fill in:

   | Field | Value |
   |---|---|
   | Name | `budminder` |
   | Repository | `ghcr.io/sirbusta/budminder:latest` |
   | Network Type | Bridge |

3. Click **Add another Path, Port, Variable, Label or Device** three times, once for each of these:

   | Config Type | Name | Container | Host |
   |---|---|---|---|
   | Port | `WebUI` | Container Port `8080` | Host Port of your choice, e.g. `8091` (any free port) |
   | Path | `Data` | Container Path `/app/data` | `/mnt/user/appdata/budminder/data` |
   | Path | `Uploads` | Container Path `/app/uploads` | `/mnt/user/appdata/budminder/uploads` |

4. Optional: switch the page to **Advanced View** (the Basic View / Advanced View switch at the top right) to see two more fields. Set **WebUI** to `http://[IP]:[PORT:8080]/` to get a WebUI link in the container's menu; Unraid replaces `[PORT:8080]` with the host port you chose, so it opens BudMinder, not whatever else uses 8080. Set **Icon URL** to `https://raw.githubusercontent.com/SirBusta/budminder/main/web/static/img/brand/budminder-256.png` for the BudMinder icon in the Docker tab.
5. Click **Apply**, then open `http://<your-server>:<host port>`.

SQLite is used automatically. To use PostgreSQL instead, see the PostgreSQL section below.

</details>

<details>
<summary><b>Download for Windows, macOS or Linux</b></summary>

<br>

Download the program for your system from the [latest release](https://github.com/SirBusta/budminder/releases/latest), put it in its own folder and run it. BudMinder starts on `http://localhost:8080` and keeps its database and photos in `data` and `uploads` folders next to the program. Set the same `BUDMINDER_…` settings below as environment variables if you need them.

- **Windows**, **Linux** and **macOS (beta):** see the step-by-step guides below.
- **From your phone:** on the same Wi-Fi, open `http://<your computer's address>:8080`, for example `http://192.168.1.108:8080`. The computer has to stay on with BudMinder running.

BudMinder has no window of its own: it runs in the background and you use it in your web browser.

#### Windows, step by step

1. **Download BudMinder.** Open the [latest release](https://github.com/SirBusta/budminder/releases/latest) page, scroll down to **Assets** and click **budminder-windows-amd64.exe**.
2. **Give it a home.** Make a folder called `BudMinder` in your user folder (`C:\Users\<you>\BudMinder`) and move the downloaded file into it. BudMinder keeps all your data (database and photos) in this folder, so don't run it from **Downloads**, and don't put it in **OneDrive** (on many PCs that includes Documents and Desktop), **Dropbox**, **Google Drive**, **iCloud Drive** or Program Files. Syncing apps copy the database while BudMinder is writing to it, which can damage it; put BudMinder's backups in the cloud instead. If you start it from Downloads or a synced folder anyway, BudMinder explains this and lets you pick a folder, then sets itself up there.
3. **Start it** by double-clicking the file. The program isn't signed, so Windows may say "Windows protected your PC": click **More info → Run anyway**. If Windows Firewall asks, allow it on **private networks** so your phone can connect.
4. **Create your account.** Your browser opens BudMinder, and a small BudMinder window explains where things are. BudMinder asks you to create a local account: choose your username and password. In the small window you can choose whether to put a BudMinder icon on the desktop; closing it (or **Got it**) sends it into the BudMinder icon by the clock, and BudMinder keeps running.
5. **From now on, use the icon.** BudMinder adds a **BudMinder** icon to your Start menu (and to your desktop, unless you unticked that). Double-click it to start BudMinder in the background (no window) and open it in your browser. You don't need to run the downloaded file again, but keep it and its folder: the icon uses the file, and your data is in the folder.
6. **To stop it,** right-click the BudMinder icon by the clock and choose **Stop BudMinder**, or click **Stop BudMinder** in the Start menu. Closing the browser or BudMinder's small window doesn't stop BudMinder: it keeps running in the background for your phone and sensors.
7. **Updating.** When a new version is out, **Update available** appears next to the BudMinder logo: click it to see what's new, then **Update now**. BudMinder backs up your database, installs the new version and restarts by itself. To update by hand instead: click **Stop BudMinder**, download the new **budminder-windows-amd64.exe**, and put it in your BudMinder folder in place of the old one (same name). Then start it with the icon as usual. Your data stays in the folder. You can also just download the new version and double-click it (from Downloads is fine): it finds your BudMinder, asks before updating it, and afterwards you can delete the download.

#### Linux, step by step

1. **Download BudMinder.** Open the [latest release](https://github.com/SirBusta/budminder/releases/latest) page and scroll down to **Assets**. Most computers need **budminder-linux-amd64**: click it to download. Only ARM computers, such as a Raspberry Pi, need **budminder-linux-arm64**. Not sure? Open a terminal and type `uname -m`: `x86_64` means amd64, `aarch64` means arm64.
2. **Give it a home.** Make a folder called `budminder` in your home folder and move the download into it. BudMinder keeps all your data in this folder, so don't run it from **Downloads**. If you start it from Downloads anyway, BudMinder explains this and lets you pick a folder, then sets itself up there.
3. **Make it runnable.** In a terminal:

   ```bash
   chmod +x ~/budminder/budminder-linux-amd64
   ```

   (Use `arm64` instead of `amd64` here and below if that's the file you downloaded.)
4. **Start it:**

   ```bash
   ~/budminder/budminder-linux-amd64
   ```

   When it's ready, a box at the end shows the address to open. BudMinder also adds a **BudMinder** icon to your desktop and app menu.
5. **Open it** at [http://localhost:8080](http://localhost:8080). BudMinder asks you to create a local account: choose your username and password.
6. **To stop it,** press **Ctrl+C** in that terminal, or close the terminal. Closing the browser doesn't stop BudMinder: it keeps running for your phone and sensors.
7. **From now on, use the icon.** Double-click **BudMinder** on your desktop or in the app menu: it starts BudMinder in the background if needed and opens it in your browser. No terminal needed, but keep the program and its folder: the icon uses the program, and your data is in the folder. To stop it, right-click the icon → **Stop BudMinder**. The first time, your desktop may ask whether to trust the new icon: allow it. If you move the `budminder` folder, start the program once from the terminal to update the icon.
8. **Start it with the computer (optional).** Instead of steps 4 and 6, let your system run it in the background:

   ```bash
   mkdir -p ~/.config/systemd/user
   cat > ~/.config/systemd/user/budminder.service <<'EOF'
   [Unit]
   Description=BudMinder

   [Service]
   WorkingDirectory=%h/budminder
   ExecStart=%h/budminder/budminder-linux-amd64
   Restart=on-failure

   [Install]
   WantedBy=default.target
   EOF
   systemctl --user enable --now budminder
   loginctl enable-linger "$USER"
   ```

   It now starts when the computer does, even before you sign in. Stop it with `systemctl --user stop budminder`, and turn this off with `systemctl --user disable --now budminder`.
9. **Updating.** When a new version is out, **Update available** appears next to the BudMinder logo: click it to see what's new, then **Update now**. BudMinder backs up your database, installs the new version and restarts by itself. To update by hand instead: stop BudMinder, replace the program file with the new download (keep the same name), and start it again. If you set up step 8, run `systemctl --user restart budminder` after replacing the file. Your data stays in the `data` and `uploads` folders. You can also just download the new version and double-click it (from Downloads is fine): it finds your BudMinder, asks before updating it, and afterwards you can delete the download.
10. **Using it from your phone** works as described above. If you've turned on the firewall (`ufw`), open the port first with `sudo ufw allow 8080/tcp`.

#### macOS, step by step

> **Beta.** The Mac version hasn't been tried on a real Mac yet. It runs in a Terminal window, with no app icon, and stopping it means closing that window. If you try it, please say how it went in the [support thread](https://forums.unraid.net/topic/201028-support-sirbusta-budminder-self-hosted-cannabis-grow-journal/) or in a [GitHub issue](https://github.com/SirBusta/budminder/issues).

1. **Download BudMinder.** First check which Mac you have: open the Apple menu → **About This Mac**. Then open the [latest release](https://github.com/SirBusta/budminder/releases/latest) page and scroll down to **Assets**. If your Mac says **Apple M1, M2, M3 or M4** (or later), click **budminder-macos-arm64** to download it; if it says **Intel**, click **budminder-macos-amd64**.
2. **Give it a folder.** In Finder, open your home folder, make a new folder called `BudMinder`, and move the download into it. Your data will live in this folder too.
3. **Make it runnable.** Open **Terminal** (in Applications → Utilities) and run these two lines:

   ```bash
   chmod +x ~/BudMinder/budminder-macos-arm64
   xattr -d com.apple.quarantine ~/BudMinder/budminder-macos-arm64
   ```

   (Use `amd64` instead of `arm64` here and below if that's the file you downloaded.) The second line tells macOS it may run a program that isn't signed by Apple. If it says "No such xattr", that's fine.
4. **Start it** by double-clicking the program in Finder. A Terminal window opens with BudMinder running in it; leave that window open. From now on, start BudMinder the same way: double-click the program in your `BudMinder` folder (or use step 8 to start it when you sign in). The first time, macOS may ask whether BudMinder may accept incoming network connections: click **Allow** if you want to use it from your phone.
5. **Open it** at [http://localhost:8080](http://localhost:8080). BudMinder asks you to create a local account: choose your username and password.
6. **To stop it,** close that Terminal window, or press **Ctrl+C** in it. Closing the browser doesn't stop BudMinder: it keeps running in that window for your phone and sensors.
7. **A shortcut (optional).** With BudMinder running, open it in Safari and choose **File → Add to Dock**, or in Chrome or Edge install it from the browser's menu (in Chrome: **Cast, save and share → Install page as app**). You get an icon that opens BudMinder in its own window. BudMinder itself still has to be running.
8. **Start it when you sign in (optional).** Open System Settings → **General → Login Items**, click **+** under "Open at Login", and choose the program in your `BudMinder` folder. It then opens in a Terminal window each time you sign in.
9. **Updating.** When a new version is out, **Update available** appears next to the BudMinder logo: click it to see what's new, then **Update now**. BudMinder backs up your database, installs the new version and restarts by itself. To update by hand instead: stop BudMinder, replace the program file with the new download (keep the same name), repeat step 3 for the new file, and start it again. Your data stays in the `data` and `uploads` folders. Or do step 3 for the new download and open it from wherever it is: it finds your BudMinder, asks before updating it, and afterwards you can delete the download.

#### Nothing is installed: it's a portable program

There's no installer, no admin rights needed, and nothing goes into Program Files or the Windows registry, so BudMinder doesn't appear in your list of installed apps. Everything it creates is a plain file you can see:

- **Its folder** (for example `C:\Users\<you>\BudMinder` or `~/budminder`), holding the program, `data` (the database), `uploads` (your photos) and `logs`.
- **Its shortcuts:** on Windows, **BudMinder** on the desktop plus **BudMinder** and **Stop BudMinder** in the Start menu; on Linux, a `budminder.desktop` file on the desktop and one in `~/.local/share/applications`.
- **A one-line note** saying where BudMinder is, so a newer download can find it and offer to update it: `%AppData%\BudMinder\home.txt` on Windows, `~/.config/budminder/home` on Linux, `~/Library/Application Support/BudMinder/home` on macOS.

**Going back to the previous version:** each update keeps the version it replaced next to the program, with `.old` added to its name, until the next update. To go back, stop BudMinder, delete the program, and remove `.old` from the old one's name. The database is backed up in `data/backups` before every update (`before-<version>-<date>.db`). If the new version changed it, also put that backup back: replace `data/isley.db` with it (this undoes anything entered since the update).

**Moving to another computer:** stop BudMinder, copy the whole folder over, and start the program there once. It makes new shortcuts.

**Removing it:** stop BudMinder, then delete its folder and its shortcuts. Deleting the folder deletes your grow journal too, so make a backup first (**Settings → Backup & data**) if you might want it later.

</details>

<details>
<summary><b>Using BudMinder away from home</b></summary>

<br>

At home, your phone reaches BudMinder at the "From your phone" address in the welcome box, for example `http://192.168.1.108:8080`. That's your computer's address on your home network, so it only works on your home Wi-Fi.

From anywhere else, the safe way is **Tailscale**, which is free for personal use. It joins your phone and your computer in a private network of your own, encrypted end to end. Nothing on your router is opened, and nobody else can reach BudMinder.

1. **Make a Tailscale account** at [tailscale.com](https://tailscale.com). You can sign in with Google, Microsoft, Apple or GitHub.
2. **Install Tailscale on the computer that runs BudMinder** and sign in:
   - Windows and macOS: download it from [tailscale.com/download](https://tailscale.com/download).
   - Linux: run `curl -fsSL https://tailscale.com/install.sh | sh`, then `sudo tailscale up` and open the link it prints.
   - Unraid: install the **Tailscale** plugin from the Apps tab, then sign in from its settings page.
3. **Install the Tailscale app on your phone** (App Store or Google Play), sign in with the same account and switch it on.
4. **Find your computer's Tailscale name.** In the phone app, or at [login.tailscale.com](https://login.tailscale.com/admin/machines), your computer is listed with a name such as `my-pc` and an address starting with `100.`.
5. **Open BudMinder on your phone** at `http://my-pc:8080` (your computer's name), or `http://100.x.y.z:8080` if the name doesn't work. Bookmark it, or add it to your home screen. It works at home too, so one bookmark is enough.

Tailscale has to stay on, on both the computer and the phone. If the page doesn't load, check that the computer is awake and BudMinder is running. On Windows, also check that Windows Firewall allows BudMinder (Windows step 3 above).

**Don't forward a port on your router to BudMinder.** That puts it on the open internet, where it's probed all day, and without HTTPS your password travels unencrypted. If you want a public web address anyway, put BudMinder behind HTTPS (see **Reverse proxy (HTTPS)** under Reference).

</details>

<details>
<summary><b>PostgreSQL instead of SQLite</b></summary>

<br>

SQLite is built in and is all most people need. If you'd rather use PostgreSQL, [`docker-compose.postgres.yml`](docker-compose.postgres.yml) starts BudMinder and PostgreSQL together. Put the database password in a `.env` file next to it:

```bash
BUDMINDER_DB_PASSWORD=choose-a-strong-password
```

```bash
docker compose -f docker-compose.postgres.yml up -d
```

To use a PostgreSQL server you already run (on Unraid, another container, or another machine), set `BUDMINDER_DB_DRIVER=postgres` and the `BUDMINDER_DB_HOST`, `BUDMINDER_DB_PORT`, `BUDMINDER_DB_USER`, `BUDMINDER_DB_PASSWORD` and `BUDMINDER_DB_NAME` settings on BudMinder.

**Moving from SQLite to PostgreSQL:** use [`docker-compose.migration.yml`](docker-compose.migration.yml). It mounts your SQLite data next to a new, empty PostgreSQL database and copies everything over on first start. Back up first, and switch to `docker-compose.postgres.yml` afterwards.

</details>

## Reference

<details>
<summary><b>Configuration settings</b></summary>

<br>

Most settings live in the app under **Settings**. These few are startup settings, set on the container (or as environment variables) because BudMinder needs them before it starts. The older `ISLEY_…` names (for example `ISLEY_DB_DRIVER`) still work.

| Setting | Default | What it does |
|---|---|---|
| `BUDMINDER_PORT` | `8080` | Port BudMinder listens on |
| `BUDMINDER_SESSION_SECRET` | made on first start | Key that keeps you logged in (32+ characters). BudMinder makes one on first start and keeps it in its database; set this only to use your own |
| `BUDMINDER_SECURE_COOKIES` | `false` | `true` sends the login cookie over HTTPS only. Turn on behind an HTTPS reverse proxy |
| `BUDMINDER_HSTS_MAX_AGE` | `0` | Above 0, tells browsers to always use HTTPS for this many seconds. Only when BudMinder is reachable over HTTPS alone |
| `BUDMINDER_HSTS_INCLUDE_SUBDOMAINS` | `false` | Adds `includeSubDomains` to the HSTS header |
| `BUDMINDER_HSTS_PRELOAD` | `false` | Adds `preload` to the HSTS header |
| `GIN_MODE` | `release` | `debug` for detailed request logs |
| `BUDMINDER_DB_DRIVER` | `sqlite` | `sqlite` or `postgres` |
| `BUDMINDER_DB_FILE` | `data/isley.db` | Where the SQLite database is kept |
| `BUDMINDER_DB_HOST` | | PostgreSQL server address |
| `BUDMINDER_DB_PORT` | `5432` | PostgreSQL port |
| `BUDMINDER_DB_USER` | | PostgreSQL user |
| `BUDMINDER_DB_PASSWORD` | | PostgreSQL password |
| `BUDMINDER_DB_NAME` | | PostgreSQL database name |
| `BUDMINDER_DB_SSLMODE` | `disable` | `require` or `verify-full` for a PostgreSQL server that uses TLS |

</details>

<details>
<summary><b>Sensors: AC Infinity, Ecowitt, Home Assistant and your own</b></summary>

<br>

**AC Infinity** (UIS controllers). BudMinder reads your controllers through AC Infinity's cloud, so the controller has to be online and on your AC Infinity account.

1. In **Settings → Integrations**, turn on AC Infinity and sign in with your AC Infinity email and password. BudMinder keeps the access token it gets back. AC Infinity logs that token out from time to time; tick **Stay signed in** and BudMinder keeps your password, encrypted, and signs in again by itself when that happens. The encryption key is kept in its own file in BudMinder's data folder (`/app/data` in Docker), not in the database or in backups.
2. In **Sensors → Manage sensors**, click **Scan AC Infinity** and add the sensors and ports you want.
3. Put each sensor in a tent and choose where it shows.

AC Infinity's sign-in only uses the first 25 characters of a password.

**Ecowitt** gateways (GW1000, GW1100, GW2000 and similar) are read directly on your network, with no cloud account.

1. In **Settings → Integrations**, turn on Ecowitt.
2. In **Sensors → Manage sensors**, click **Scan Ecowitt** and enter the gateway's IP address.
3. Add the sensors you want and put each in a tent. Scan each gateway separately if you have more than one.

**Home Assistant.** Any numeric sensor in Home Assistant can be added: temperature, humidity, CO₂, soil moisture, light, EC and more. BudMinder only reads values; it never changes anything in Home Assistant.

1. In Home Assistant, create a separate user for BudMinder (**Settings → People → Add person**, allow login, *not* an administrator).
2. Log in to Home Assistant as that user, open your profile → **Security** → **Long-lived access tokens**, and create a token. Copy it; Home Assistant shows it only once.
3. In BudMinder, **Settings → Integrations → Home Assistant**: turn it on, enter Home Assistant's address with its port (for example `http://192.168.1.20:8123`), paste the token and click **Test connection**.
4. In **Sensors → Manage sensors**, click **Scan Home Assistant**. Sensors useful for growing are listed first with their current readings; batteries, power and the like appear with **Show all**. Add the ones you want and put each in a tent.

If Home Assistant restarts or is offline for a while, BudMinder logs it and picks the readings back up when it returns.

Readings are collected at the polling interval set in Settings (every 60 seconds by default).

**Your own sensors:** if you build your own (an ESP32 board, for example), you can program it to send readings through the API below.

</details>

<details>
<summary><b>API</b></summary>

<br>

Create an API key in **Settings → Integrations** and send it as an `X-API-KEY` header. The key is shown once; making a new one replaces the old one.

**Send a reading:** `POST /api/sensors/ingest`, one reading per request. A new sensor is created the first time its `source`, `device` and `type` are seen.

```json
{
  "source": "custom",
  "device": "ESP32 tent probe",
  "type": "temperature",
  "value": 25.5,
  "name": "Canopy temp",
  "new_zone": "Tent 1",
  "unit": "°C"
}
```

**Stream overlay:** `GET /api/overlay` returns every growing plant (with its linked sensor readings) and all sensors grouped by source, tent and type. Made for live-stream overlays such as an OBS browser source.

**Sensor history:** `GET /sensorData?sensor=5&minutes=1440` returns readings for one sensor, by `minutes`, or by `start` and `end` dates (`YYYY-MM-DD` or RFC 3339). It needs a login unless guest view is on.

</details>

<details>
<summary><b>Backup and restore</b></summary>

<br>

**From the app:** **Settings → Backup & data**. Choose how much sensor history to include and whether to include photos, then create the backup. It runs in the background and appears in the list to download. Download it to keep it somewhere safe: backups stored only on the same computer are lost if that computer fails. A backup includes your saved connection details, such as the AC Infinity and Home Assistant tokens, so keep backup files private.

**Restoring** replaces everything in the current BudMinder with the backup. On SQLite you can also download and upload the database file itself under **SQLite file transfer**, which is the quickest way to copy a SQLite setup.

Uploads are limited to 5 GB by default; change it in the restore section (minimum 100 MB). Backups are always full backups, and a large backup with photos needs about as much free memory as its size while restoring. For automatic backups, call `POST /settings/backup/create` on a schedule, or back up the Docker volumes.

**From Docker:** everything BudMinder stores is in its two volumes: the database volume (`/app/data`, or PostgreSQL's own volume) and the uploads volume (`/app/uploads`, your photos and camera snapshots). For SQLite, stop the container before copying the database file.

</details>

<details>
<summary><b>Reverse proxy (HTTPS)</b></summary>

<br>

**Nginx**

```nginx
server {
    listen 443 ssl;
    server_name budminder.example.com;

    ssl_certificate     /etc/letsencrypt/live/budminder.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/budminder.example.com/privkey.pem;

    client_max_body_size 20M;  # photo uploads

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**Traefik** (labels on the `budminder` service)

```yaml
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.budminder.rule=Host(`budminder.example.com`)"
  - "traefik.http.routers.budminder.entrypoints=websecure"
  - "traefik.http.routers.budminder.tls.certresolver=letsencrypt"
  - "traefik.http.services.budminder.loadbalancer.server.port=8080"
```

BudMinder trusts `X-Forwarded-For` from private network addresses, so a proxy on your network is handled correctly. Behind HTTPS, also set `BUDMINDER_SECURE_COOKIES=true`.

</details>

## Credits and license

BudMinder is based on [Isley](https://github.com/dwot/isley) by dwot. Strain data comes from [StrainCompass](https://straincompass.com) and [CannaDB](https://cannadb.org). Icons are from [Font Awesome Free](https://fontawesome.com), and the logo is set in [Outfit](https://fonts.google.com/specimen/Outfit).

Found a bug or have an idea? [Open an issue](https://github.com/SirBusta/budminder/issues).

Released under the [MIT license](LICENSE).
