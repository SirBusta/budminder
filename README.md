<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="web/static/img/brand/budminder-wordmark-dark.svg">
  <img src="web/static/img/brand/budminder-wordmark.svg" alt="BudMinder" width="340">
</picture>

**A self-hosted cannabis grow journal.** Daily notes and photos for every plant, live sensors from AC Infinity, Ecowitt or Home Assistant, and your seed vault, all on your own hardware.

[![Latest release](https://img.shields.io/github/v/release/SirBusta/budminder?color=16a34a&label=release)](https://github.com/SirBusta/budminder/releases)
[![Docker image](https://img.shields.io/badge/docker-ghcr.io%2Fsirbusta%2Fbudminder-16a34a?logo=docker&logoColor=white)](https://github.com/SirBusta/budminder/pkgs/container/budminder)
[![License: MIT](https://img.shields.io/badge/license-MIT-16a34a)](LICENSE)

[Install](#install) · [Tour](#tour) · [Releases](https://github.com/SirBusta/budminder/releases)

</div>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/screenshots/plant-dark.webp">
  <img src="docs/images/screenshots/plant.webp" alt="A plant's page in BudMinder: the stage track from germination to harvest, a notebook with one entry per day, and the three photos taken on the selected day">
</picture>

<p align="center"><sub>Every plant gets a stage track from seed to harvest and a notebook with one entry per day, shown next to that day's photos.</sub></p>

## Why BudMinder?

**Your grow, on your hardware.** BudMinder is a self-hosted grow journal designed to run entirely on your own machine or home server. No cloud accounts, no corporate tracking, and no treating your cultivation logs as a commodity. What you record stays on your own hardware.

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

Every plant has its own page and notebook. The stage track shows where it is from germination to harvest, how long it spent in each stage and when to expect harvest, based on the strain's flowering time; click a stage to move the plant along. Each day you log is one row: water and feed amounts, training, height and notes, with that day's photos right beside it. Filter by activity, search your notes, switch between weeks and a plain list, or see every plant's notebook at once. Log for a whole tent in one go, check the tent camera live, and export the notebook to CSV or Excel.

<img src="docs/images/screenshots/plant-pm1-dark.webp" alt="Permanent Marker #1's page in dark mode: the stage track at Flower week 2 with an estimated harvest of Nov 30 to Dec 7, the notebook grouped by week with water amounts, heights and notes, and the two photos from Sep 28 shown beside the selected day"><br>
<sub>Shown in dark mode.</sub>

### Today

Open BudMinder and see what needs doing: plants you haven't watered or fed in a while, seeds getting old, and every tent's live temperature, humidity, VPD and CO₂ next to its camera and plants. Log water or feeding right from a plant's card.

<img src="docs/images/screenshots/home.webp" alt="The Today page: a needs-attention list, then each tent with its live readings, camera snapshot and plant cards">

### Environment

Every sensor in a tent on one timeline, from the last hour to the last week or any range you pick. Click anywhere on a chart and a marker line shows what every sensor read at that moment; drag it to scrub through the day. Works with AC Infinity controllers, Ecowitt gateways and any sensor in Home Assistant. If you build your own sensors, a simple API lets you send in their readings.

<img src="docs/images/screenshots/environment.webp" alt="The Environment page for a tent: current readings, then temperature, humidity and VPD charts for the last 24 hours with a marker line showing the readings at 3:25 AM">

### Seed Vault

The seeds you have on hand, how old they are and where you keep them, with a warning when a batch passes two years. Use one, restock, or start a grow straight from the list, and keep a wish list of strains you want next.

<img src="docs/images/screenshots/strains.webp" alt="The Seed Vault in list view, oldest seeds first, showing each strain's lineage, type, flowering time, seed count, seed age and grows">

### Strain pages

Everything about a strain in one place: every time you've grown it with yields and flowering times, its family, potency and growing facts, and the photos from your grows. Import a new strain from StrainCompass or CannaDB and most of this fills itself in.

<img src="docs/images/screenshots/strain-grape.webp" alt="The Grape OG strain page: four harvested plants with yields, the strain's family, seed vault and strain facts, and photos from the grows">

### Smoke reports

A tasting journal for the flower you grow or buy. Score bud quality, taste, smell and effects from 1 to 10 and get an overall score, weighted the way you like. Pick effects, side effects, smells and tastes from lists or type your own, and say whether you'd grow or buy it again. For bought flower, look the strain up, add the dispensary and price, and BudMinder works out the price per gram. Add a photo by upload, from a web address, or from the plant's journal. Every report gets its own page, and plants and strains show their latest scores.

<img src="docs/images/screenshots/smoke-report.webp" alt="A smoke report for home-grown Grape OG: a photo of two dried buds, an overall score of 7.8 out of 10, THC, CBD and cure time, top effects and smells, bars for the bud quality, taste, smell and effects scores, and the grow details BudMinder filled in from the plant">

### Home Assistant

Already have sensors in Home Assistant? Connect BudMinder once and every sensor Home Assistant knows about (Govee, SwitchBot, Inkbird, Zigbee, ESPHome and more) can be added to a tent, right next to AC Infinity and Ecowitt readings. A scan lists the sensors useful for growing with their live readings; add the ones you want. BudMinder only reads values; it never changes anything in Home Assistant.

<img src="docs/images/screenshots/ha-scan.webp" alt="Scan Home Assistant results: seven grow sensors such as tent temperature, humidity, CO2, VPD, light level and soil moisture, each with its current reading, a tent to put it in and an Add button, plus a filter box and a Show all switch for batteries and power">

### More

<table>
<tr>
<td width="50%" valign="top"><b>Plants</b><br>Every plant with its stage, progress to harvest, and when it was last watered and fed.<br><br><img src="docs/images/screenshots/plants.webp" alt="The Plants page listing six growing plants with stage, progress bar and days since watering and feeding"></td>
<td width="50%" valign="top"><b>Log activity</b><br>Water, feed, train or add a note for one plant or a whole tent at once, with the amount given.<br><br><img src="docs/images/screenshots/log-activity.webp" alt="The Log activity window with plants, date, activity types, water amount and a note"></td>
</tr>
<tr>
<td valign="top"><b>Manage sensors</b><br>Scan AC Infinity, Ecowitt and Home Assistant for sensors, name them, put each in a tent and choose where it shows.<br><br><img src="docs/images/screenshots/manage-sensors.webp" alt="The Manage sensors page listing a tent's sensors with their current readings"></td>
<td valign="top"><b>Settings</b><br>Grouped and searchable, and every change saves as you go.<br><br><img src="docs/images/screenshots/settings.webp" alt="The Settings page with timezone, units, language, theme, guest view and password"></td>
</tr>
</table>

### On your phone

<p align="center">
<img src="docs/images/screenshots/phone-home.webp" alt="The Today page on a phone" width="260">&nbsp;&nbsp;&nbsp;
<img src="docs/images/screenshots/phone-plant.webp" alt="A harvested plant's page on a phone in dark mode" width="260">
</p>

### And also

- One-click strain import from StrainCompass, with THC and CBD ranges, effects, terpenes and parents
- Smoke reports for flower you grew or bought, sortable by score, price per gram or date
- Hide the smoke report fields you never use, and choose how much each score counts
- Imperial or metric units for heights, yields and temperatures
- Everything is served from your own server: no outside fonts, scripts or trackers
- Journal export to CSV or Excel (PDF coming soon)
- Docker images for regular PCs and ARM (Raspberry Pi, newer Macs), plus ready-to-run programs for Windows, macOS and Linux
- Light and dark themes, in English, German, Spanish and French
- A tent camera: live view plus automatic snapshots
- Guest view for sharing, read-only
- SQLite or PostgreSQL

<sub>Screenshots use a demo data set. The sensor history is a month of real readings from an AC Infinity controller in a 3x3 tent.</sub>

## Install

BudMinder runs in Docker. You need [Docker](https://docs.docker.com/get-docker/) with Compose.

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
> The first login is **admin** / **budminder**. BudMinder asks you to change the password on first launch.

To update later, run `docker compose pull` and then `docker compose up -d`. Your data stays in the two volumes.

<details>
<summary><b>Unraid</b></summary>

<br>

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
<summary><b>Without Docker (Windows, macOS, Linux)</b></summary>

<br>

Download the program for your system from the [latest release](https://github.com/SirBusta/budminder/releases/latest), put it in its own folder and run it. BudMinder starts on `http://localhost:8080` and keeps its database and photos in `data` and `uploads` folders next to the program. Set the same `BUDMINDER_…` settings below as environment variables if you need them.

- **Windows:** the program isn't signed, so Windows may say "Windows protected your PC". Click **More info → Run anyway**. When Windows Firewall asks, allow it on **private networks** so other devices can connect.
- **macOS:** make it runnable with `chmod +x budminder-macos-*`. The first time, right-click the file → **Open** (or allow it in System Settings → Privacy & Security), because it isn't signed by Apple.
- **Linux:** `chmod +x budminder-linux-*`, then run it.
- **From your phone:** on the same Wi-Fi, open `http://<your computer's address>:8080`, for example `http://192.168.1.108:8080`. The computer has to stay on with BudMinder running.

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

**Restoring** replaces everything in the current BudMinder with the backup. On SQLite you can also skip sensor history to restore faster, or download and upload the database file itself under **SQLite file transfer**, which is the quickest way to copy a SQLite setup.

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
