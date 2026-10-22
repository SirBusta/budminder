# Changelog

All notable changes to BudMinder are documented in this file. Versions before
1.0.0 are releases of [Isley](https://github.com/dwot/isley), the project
BudMinder is based on.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Maintainer notes

- Edit the `[Unreleased]` section as part of each dev-branch commit that lands
  a user-visible change. One-liners are fine; the idea is "describe it while
  the context is fresh," not "write perfect prose."
- When cutting a release, run `scripts/cut-release.sh X.Y.Z` on a
  `release/vX.Y.Z` branch. The script renames `[Unreleased]` to
  `[X.Y.Z] - YYYY-MM-DD` and seeds a fresh empty `[Unreleased]` block.
- The GitHub release workflow extracts the matching version's section from
  this file and uses it as the release body. Keep the headings exact.
- Historical releases prior to `0.1.42` are catalogued on GitHub Releases;
  they predate this file and were generated from commit logs.

## [Unreleased]

### Added

### Changed

### Fixed

### Deprecated

### Removed

### Security

## [1.5.1] - 2026-10-22

### Changed
- **Plants, Harvested:** each plant shows the date it was harvested (the day it
  moved to Drying), and the list can be sorted by harvest date, newest or
  oldest first. The sort menu now only offers what fits each tab.
- **README:** every picture has a magnifying-glass badge and opens the
  full-size screenshot.

### Fixed
- **Strain lineage:** parent strains carried over from older data could be
  split at a bracket, like "(Creme de la Chem" and "Mango Smile)", and their
  StrainCompass search found nothing. They are now rejoined into one parent
  when BudMinder starts and after a restore, and the search link ignores
  brackets.
- **Backup & data:** removed an outdated warning that restoring sensor data
  into SQLite is very slow; restores have been fast since 1.0.1.

## [1.5.0] - 2026-10-22

### Added
- **A short guided tour for new users.** Four tips (Strains, Stage track,
  Notebook, Sensors) with pictures whose lines fill in one at a time, a ring
  around the spot each line describes, and short clips that play one step at
  a time. A new install is asked once on Today whether it wants the tour;
  existing installs are never asked. Take it again any time, or jump to one
  tip, from the new "Take the tour" card on Help's Welcome page or the link at
  the top of Help's side menu. In English, German, Spanish and French.
- **The live demo offers the tour** to each visitor once, on Today. The answer
  stays in the visitor's browser.

### Changed
- **Demo: the Plants list opens "Oldest first"**, so the two showcase plants
  are at the top. Visitors can still pick another sort.
- **README:** links the live demo and mentions the tour.

### Fixed
- **Settings:** the Integrations section's description now mentions Home
  Assistant.

## [1.4.0] - 2026-10-21

### Changed
- **Demo mode keeps its grow at the same point.** Once a day the demo moves its
  plant, notebook, photo, seed and smoke report dates forward by the days that
  passed, so visitors always see the same stage of the grow ("Day 76, watered
  yesterday") without the data ever needing a refresh.
- **Feeding no longer asks for a water amount.** The feed's details go in the
  day's note. An activity that is both watering and feeding still asks, and
  amounts already recorded stay and can still be edited.
- **Demo: visitors can type notes** (up to 500 characters) instead of picking
  from sample notes, since each visitor works in a private copy; changes are
  limited to 15 a minute per visitor, and the banner says that changes reset
  after an hour of inactivity.

### Fixed
- **Plant page: ticking Water and Feed showed two "Water amount" boxes.** Each
  measurement now appears once, as on the Today page.

### Security
- **Photos no longer carry hidden location data.** Every photo BudMinder takes
  in (plant photos, smoke report photos, seed-pack pictures, the logo, and
  photos restored from a backup) is stored without its metadata: no GPS
  location, camera make or serial, dates, editing history or comments, and
  nothing hidden after the end of the image (such as a phone's motion photo).
  Only the color profile and the orientation are kept, and the picture itself
  is untouched. Uploads must now be complete, readable images.

## [1.3.0] - 2026-10-20

### Added
- **Demo mode** (`BUDMINDER_DEMO=1`), for running a public demo: visitors are
  signed in automatically, text comes from sample choices, uploads, exports,
  settings and integrations are off, and the server checks every change. Each
  visitor gets a private copy of the demo on their first change, erased after
  an hour idle. Off unless you turn it on (SQLite only).
- **Training and Defoliate activities** come standard, with their own icons
  and colors. Installs that already have their own training or defoliation
  activity keep theirs, without a duplicate.

### Fixed
- **Today: every quiet sensor gets a warning.** A sensor that stops reporting
  now shows in Needs attention even when it isn't set to show in the tent
  summary on the Home page. Hidden sensors still stay out.

### Security
- **Guest view can no longer create images.** With guest view on, visitors who
  weren't signed in could reach the photo-decorating action, which saves a
  new image on the server. It now needs a sign-in, like every other change.
- **Stricter page security policy.** Scripts can no longer turn text into code
  (`unsafe-eval` removed; nothing in BudMinder used it), and the policy now
  also blocks plugins, embedding BudMinder in other sites, and forms or base
  links pointing elsewhere. Found with an OWASP ZAP scan.

## [1.2.0] - 2026-10-19

### Added
- **Help manual.** A **Help** button in the top menu opens a manual inside
  BudMinder, at the section for the page you're on. Sections on the left,
  a search box, and pictures with numbered circles that match the text.
  Pictures follow the light or dark theme; click one to see it full size.
  Written in English for now.

## [1.1.0] - 2026-10-16

### Added
- **Smoke reports.** A tasting journal for flower you grew and flower you
  bought, with its own **Smoke** page in the top menu (cards or a sortable
  list).
  - Score bud quality, taste, smell and effects from 1 to 10. The overall
    score is a weighted average, and you choose the weights in Settings.
  - Pick effects, side effects, smells, tastes and bud qualities from lists,
    or type your own. Also how it was smoked, and whether you'd grow or buy
    it again.
  - From one of your plants, BudMinder fills in the strain, harvest date,
    tent, days to harvest, dry and cure time, THC, a light from the tent's
    AC Infinity ports, and what you used last time in that tent.
  - For bought flower: look the strain up in your strains or on
    StrainCompass, change anything, add the dispensary, price (price per gram
    is worked out) and batch, and optionally add the strain to your Strains.
  - Photos: upload them, give a web address (BudMinder keeps its own copy),
    or pick them from the plant's journal. The first one is the main photo.
  - Each report has a product-style page with the plant's reports over time.
    Plants show a marker with their latest score, the plant page gets a
    "Smoke" step after its stages, and a strain's page lists its reports.
  - Settings → Grow setup → Smoke reports: score weights, and switches to
    hide fields you never use.

### Fixed
- The plant page no longer logs a server error when no photo logo was ever
  uploaded (the logo list is now just empty).

## [1.0.1] - 2026-10-15

### Added
- **AC Infinity "Stay signed in".** AC Infinity logs BudMinder out from time
  to time ("Login Expired"). With this ticked in the AC Infinity sign-in
  window, BudMinder keeps your AC Infinity password, encrypted, and signs in
  again by itself when that happens, so readings keep coming. The key is
  kept in its own file in the data folder, not in the database or in
  backups. "Forget saved password" turns it off.
- **PUID / PGID** settings for the Docker image. Set both, for example 99 and
  100 on Unraid, and BudMinder's data, uploads and logs folders belong to
  that user and group, and BudMinder runs as them. Without them nothing
  changes.

### Changed
- **Add Plant now starts from your strains.** Plants get their strain from
  the Strains page, where the strain's details are looked up and your seeds
  are tracked, instead of a bare "Add New Strain" with only a name and
  breeder.
  - A note at the top of Add Plant links to the Strains page. It's a
    warning when no strains exist yet.
  - Typing a strain you haven't added says so, with a link that opens the
    Strains page already searching for it. It also suggests close matches
    for typos ("Did you mean Blue Dream?").
  - After the strain is saved, a "Back to Add Plant" button returns you with
    the strain chosen and everything you'd filled in still there.
  - Edit plant works the same way.
- **Honest connection status for AC Infinity, Ecowitt and Home Assistant.**
  Manage sensors, Settings and the Today page now show what's really
  happening: connected, sign-in expired, not responding, or no new readings
  since a given time, with the service's own message. Before, AC Infinity
  showed "Connected" whenever a token was saved, even if it had stopped
  working weeks earlier.
- **Deleting a breeder is safe now.** It used to delete every strain and plant
  from that breeder as well. Now only a breeder none of your strains use can
  be deleted. Settings → Grow setup shows how many strains use each breeder,
  and for one in use, Edit Breeder explains why Delete is off and that it can
  still be renamed.
- **Family diagram on a strain's page:** a parent that isn't in your strains
  now opens a StrainCompass search for it in a new tab, instead of an old
  "add a strain" page. Parents you have still open your own strain page, now
  with a small StrainCompass link on every strain (it used to appear only on
  strains imported from StrainCompass). With StrainCompass lookups turned off,
  those parents are shown as plain names.
- Signing in to AC Infinity now confirms it worked.
- "Are you sure?" questions use BudMinder's own dialog everywhere instead
  of the browser's plain pop-up, with a red Delete button for deletions.
- In the photo viewer and the Decorate window, Delete photo (and Decorate)
  moved below the photo, away from the close button, so it's hard to hit
  by mistake on a phone.
- In a plant's journal, the photo icon is teal instead of gray, so it no
  longer looks like the Note icon, especially in dark mode.
- Settings → Appearance no longer shows the empty "Colors" card; it comes
  back when color schemes are built.

### Fixed
- AC Infinity CO2 readings were 100 times too low (770 ppm showed as 8).
  CO2 is now read correctly and your existing CO2 history is corrected
  automatically on upgrade.
- AC Infinity refusing the saved sign-in was ignored, so readings silently
  stopped. It's now logged with AC Infinity's reason and shown as
  "Sign-in expired".
- Restoring a backup with a long sensor history is much faster (a backup
  with 1.3 million readings went from hours to under a minute).
- Backups made in Docker recorded their version as "unknown".
- The chart hint on the Environment page still mentioned zooming, which was
  removed.
- Everyone was logged out whenever BudMinder restarted unless
  `BUDMINDER_SESSION_SECRET` was set. BudMinder now makes its own key on first
  start and keeps it in its database, so logins survive restarts.
- The GitHub Release description now shows the right version's notes.
- The AC Infinity sign-in window's Password label was blank.
- Delete plant on the Edit plant page opened an empty dialog whose button
  did nothing, so the plant couldn't be deleted from there.
- Several labels were blank (Settings: breeder name, Edit breeder, Edit
  stream, Delete stream) and some messages showed internal names such as
  "api_backup_deleted" instead of a sentence. A new automatic test checks
  that every piece of text exists in all four languages.
- VPD was wrong for a temperature sensor whose unit was spelled in lower
  case ("celsius"): the temperature was read as Fahrenheit.
- Decorate's custom text boxes sat greyed out, and nothing said they only
  work after choosing "Custom" in the menu above. They now appear only when
  "Custom" is chosen, ready to type in.
- Choosing a strain or tent and then typing something else over it kept the
  earlier choice, so a plant could be saved with a strain other than the one
  shown. Typing an exact strain name without clicking it in the list now
  selects it.

### Security
- Updated libraries: security fixes in Go's crypto, image and text
  packages, a newer SQLite driver, and Go 1.27.1 for the Docker image.

## [1.0.0] - 2026-10-14

The first release under the name BudMinder.

### Added
- **Home Assistant sensors:** connect BudMinder to Home Assistant in
  Settings (address and a long-lived access token, with Test connection),
  then scan and add any of its numeric sensors from Manage sensors. Govee,
  SwitchBot, Inkbird, Zigbee, ESPHome and every other sensor Home Assistant
  supports can now sit in a tent next to AC Infinity and Ecowitt readings.
  Read-only: BudMinder never changes anything in Home Assistant.
- **Today page:** what needs attention, every tent's live readings, its camera
  and plant cards, with water and feed logging from each card.
- **Plant page with a daily notebook:** one entry per plant per day, with that
  day's activities, measurements (water amount, height) and photos side by
  side; a stage track from germination to harvest with estimated harvest
  dates; an "All plants" view; notebook export to CSV or Excel.
- **Seed Vault and Wanted list:** seed counts, age (with a warning past two
  years), seed type and storage location; − / + one seed, Restock, and start a
  grow from the list; sort by oldest seeds first.
- **Strain pages:** every grow of a strain with yields and flowering times,
  the strain's family, potency, effects, terpenes and photos from your grows.
- **StrainCompass lookup:** search and import strains with THC/CBD ranges,
  effects, flavors, terpenes, medical uses and parents, reviewed before saving.
  StrainCompass and CannaDB lookups are on by default.
- **Environment page:** every sensor in a tent on one timeline (1 hour to 7
  days or a custom range); click or drag a marker line to read all sensors at
  one moment. **Manage sensors** page to scan, add, name and place sensors.
- **Settings** rebuilt: grouped, searchable, saves as you go.
- Imperial or metric units for strain height and yield, and sensor temperatures.
- Breeder type-ahead seeded with about 1,200 breeder names.
- Docker images for amd64 and arm64 on GitHub Container Registry, and
  ready-to-run programs for Windows, macOS and Linux on each release.

### Changed
- New name, logo and icons. New installs log in with admin / budminder (you
  are asked to change it at first login).
- Every page redesigned, with a new top menu and dark mode throughout.
- Everything is served from your own server; pages no longer load fonts or
  scripts from other sites.
- Download names start with `budminder-` (backups, database, notebook export).
  Older `isley-backup-*.zip` files still restore.
- Startup settings are now named `BUDMINDER_…` (for example
  `BUDMINDER_DB_DRIVER`). The older `ISLEY_…` names still work, and the SQLite
  file stays `data/isley.db`, so existing setups keep working.

### Fixed (problems carried over from Isley)
- "Database is locked" errors when saving activities, caused by two slow
  background jobs holding the database.
- Saving sensor readings got slower and slower on SQLite as the history grew.
- Dates shown a day early for time zones west of UTC.
- Pages failing to load for plants or strains missing a strain, tent or
  breeder.
- The estimated harvest date could show "0000-12-31".
- Sensor readings of exactly 0 sent through the API were rejected; they are
  now stored.
- Strain names with an apostrophe showed stray backslashes.
- Importing the same strain from CannaDB for a second breeder overwrote the
  first one.
- Typing a breeder's name could create a duplicate breeder.
- New strains didn't appear on the Strains list until the page was reloaded.
- Adding a plant together with a brand-new strain could fail.
- Changing an activity's type (for example from Note to Water) wasn't saved.
- The Log activity window opened with the previous note already filled in.
- Errors that crashed a page weren't recorded in Settings → Logs.
- The Restore Backup card showed raw HTML code instead of formatted text.
- Backups now include strain effects, flavors, terpenes, medical uses and the
  daily notebook.

## [2.3.0] - 2026-06-23

### Added
- EcoWitt `ch_ec` support: WH52 soil sensor reporting soil moisture, soil
  temperature, and electrical conductivity (converted to mS/cm), registered
  per channel.

### Changed

### Deprecated

### Removed

### Fixed
- Deleting a sensor that had hourly-rollup data failed with "Failed to delete
  sensor". `DeleteSensorByID` now purges `sensor_data_hourly` and
  `rolling_averages` rows alongside `sensor_data` within a single transaction,
  so the foreign key on `sensor_data_hourly` no longer blocks the delete
  (#206).

### Security

## [2.2.0] - 2026-06-18

### Added
- CannaDB strain import. When enabled under Settings → "CannaDB Strain Import",
  the strain library gains an "Import from CannaDB" button that searches the
  public CannaDB database (cannadb.org) by name and one-click imports a chosen
  strain — name, breeder, indica/sativa lean, autoflower, descriptions, cycle
  time, source URL and parent lineage — into the local library. Records are
  keyed on the CannaDB AT-URI so re-importing updates in place instead of
  duplicating, and imported strains show a "View on CannaDB" link to the full
  record (THC/CBD, yield, terpenes, effects and other fields Isley doesn't
  store locally). The integration is read-only, unauthenticated, and honors
  CannaDB's rate limits with backoff. Disabled by default. An optional API-URL
  override is available for self-hosted CannaDB endpoints.
- Per-zone derived Vapor Pressure Deficit (VPD). When a zone is configured with
  a leaf-temp offset (°F) plus temperature and humidity source sensors, Isley
  computes canopy VPD each poll cycle (Tetens formula) and stores it as a
  regular `derived` sensor reading, so it both displays on the plant page and
  graphs over time. The plant-page VPD chip is colored by the plant's current
  growth stage's ideal range — green (in range), blue + up-arrow (too low),
  red + down-arrow (too high). New per-stage ideal-range editor under
  Settings > Customization (seeded Seedling 0.4–0.8, Veg 0.8–1.2,
  Flower 1.2–1.5 kPa) and new per-zone VPD inputs in the zone edit modal.
  Adds migration 019. (PR #187, contributed by @evelinwa)

### Changed
- CI/build dependency bumps:
  - `actions/checkout` 4 → 6
  - `actions/setup-go` 5 → 6
  - `actions/github-script` 7 → 9
  - `docker/setup-buildx-action` 3 → 4
  - `crazy-max/ghaction-import-gpg` 6 → 7
  - Docker runtime base `alpine` 3.23 → 3.24

### Deprecated

### Removed

### Fixed
- The activity log's empty-state message now centers correctly, matching the
  plants/strains/sensors empty states (the `.activities-empty` block was
  missing the flex centering rules). (PR #185, contributed by @ababoude)
- Completed the French (`fr`) translations for the activity log / notebook /
  export strings (issue #148), which were still placeholder `NEEDS_TRANSLATION`
  values. (PR #184, contributed by @ababoude)

### Security
- Bump golang.org/x/crypto to 0.53.0
- Bump golang.org/x/text to 0.38.0
- Bump golang.org/x/image to 0.42.0
- Bump modernc.org/sqlite to 1.52.0

## [2.1.0] - 2026-06-06

### Added
- EcoWitt sensor support now covers `ch_aisle` (WH31/WN31 multi-channel
  temperature & humidity) and `common_list` outdoor weather-station sensors
  (WH32/WN32, WS80, WH65, WS90 — outdoor T&H, wind, rain, UV, barometric
  pressure, light). These keys appeared in the gateway's `/get_livedata_info`
  response but were previously dropped on scan, so users running outdoor or
  greenhouse stations saw nothing register. Includes defensive handling for
  mixed hex/decimal sensor-ID formats across firmware versions and for
  embedded-unit / dash-placeholder values. (PR #176, contributed by
  @evelinwa)
- The Plants view auto-switches to the Harvested tab on initial load when the
  Living tab is empty — handy after wrapping up a run and inventorying
  harvested plants only. (PR #170, contributed by @anon11215)

### Changed
- The add/edit strain forms now surface the server's validation message in the
  failure toast instead of a generic "update failed" / "Failed to add strain",
  so the offending field is actionable.
- Removed an inconsistent `html.EscapeString` on the `description` field
  returned by `/strains/in-stock` and `/strains/out-of-stock`. The list pages
  don't render the description, but the pre-escape would have seeded a
  double-escape spiral for any consumer that round-tripped the value back
  through a save.
- GitHub Actions release pipeline bumps:
  - `actions/upload-artifact` 4 → 7
  - `sigstore/cosign-installer` 3.5.0 → 4.1.1
  - `softprops/action-gh-release` 2 → 3
  - `docker/setup-qemu-action` 3 → 4
  - `docker/build-push-action` 6 → 7

### Deprecated

### Removed

### Fixed
- #146 (strain regression): v0.2.0's new strain URL validation rejected
  pre-existing URLs stored without an `http(s)://` scheme (common before
  validation existed). Because the edit modal sends the saved URL back
  verbatim, editing **any** field of such a strain — and adding a strain with
  a schemeless URL — failed with a generic `update_error` /
  `Failed to add strain` toast. Schemeless URLs are now normalized
  (`https://` prepended) on add/update and persisted clean; dangerous schemes
  (`javascript:`, `ftp://`, …) are still rejected. The `description` /
  `short_desc` length caps were also relaxed so legacy long free text no
  longer blocks updates.
- `UpdateStrainHandler` now refreshes the in-memory strain cache after a
  successful update — it regressed in the v0.2.0 ConfigStore refactor and the
  UI showed stale strain data until the next restart.
- Follow-up to #146 (strain debugging): the dedicated edit page at
  `/strain/:id/edit` was still showing a generic "Update failed" toast even
  after the previous round of validation work, because `strain-edit.js` was
  missed when `strain-add.js` and the (unused) edit modal were updated to
  parse the server's JSON `error` body. The live editor now surfaces the
  actual validation message the same way.

### Security
- Bump golang Docker base to 1.26.4-alpine3.23
- Bump golang.org/x/crypto to 0.52.0
- Bump golang.org/x/text to 0.37.0
- Bump golang.org/x/image to 0.41.0
- Bump github.com/quic-go/quic-go to 0.59.1
- Bump modernc.org/sqlite to 1.50.1

## [0.2.0] - 2026-04-29

### Added
- Fixed an ordering bug when uploading multiple plant images at once
  where descriptions and dates could be paired with the wrong image. (PR #157)
- The plant detail page now has prev/next arrows in the hero and responds to ArrowLeft / ArrowRight keyboard navigation, cycling within the plant's current top-level state (Living / Harvested / Dead). (PR #158)
- Activities can now be flagged as "watering" and/or "feeding". The built-in "Water" and "Feed" activities are migrated to set the matching flag, and the dashboard "days since last watering / feeding" calculations now use the flags rather than hardcoded names — so renaming or translating the built-in activities no longer breaks tracking. (PR #160)
- Activities can now be linked to metrics. When an activity type has linked metrics, the relevant measurement fields appear on the activity-entry form (single-plant, multi-plant, and edit modals), so values like fertilizer amount or pH can be captured alongside the activity rather than buried in the notes. Required vs. optional metrics are configurable per activity in Settings, and recorded measurements stay linked to the activity that produced them. (PR #161)
- Extensive Testing overhaul, dramatically improved test coverage
- New `ISLEY_SECURE_COOKIES` documented in the README env-var table — set to `true` when fronting Isley with a TLS reverse proxy.
- `dependabot.yml` now also tracks `docker` and `github-actions` ecosystems so base-image and action bumps surface as PRs instead of stale pins.
- New `ISLEY_HSTS_MAX_AGE`, `ISLEY_HSTS_INCLUDE_SUBDOMAINS`, and `ISLEY_HSTS_PRELOAD` environment variables documented in the README — opt-in `Strict-Transport-Security` for HTTPS-only deployments.
- `strain_lineage` now travels through the SQLite→PostgreSQL bootstrap migration tool (`model/sqlite_to_postgres.go`), so users migrating an existing SQLite install with lineage data no longer silently lose it on the Postgres side.

### Changed
- The activities page now matches the look and feel of the plants, strains, and sensors pages: in-page search, filter dropdowns, sortable column headers, and a result count, with no full-page reload between filter changes. CSV and XLSX export honor the active filters. Behind the scenes, the `/activities/list` JSON endpoint now accepts a larger `page_size` so the page can render the full activity log client-side. Shared CSS for the controls bar and table styling across the four list views was deduplicated. (PR #159)
- `/health` now pings the database with a 2-second timeout and returns 503 when the DB is unreachable, so the Dockerfile HEALTHCHECK no longer reports a container with an offline backend as healthy.
- The shipped `docker-compose.{sqlite,postgres,migration}.yml` files now resolve the Isley image as `dwot/isley:${ISLEY_VERSION:-latest}` so deployments can pin a specific release without editing the compose file.
- `ISLEY_DB_SSLMODE` README description corrected to show the actual `disable` default and the recommended values for TLS-enforcing Postgres hosts.
- AC Infinity / EcoWitt scan handlers now surface upstream network and parsing failures as `502 Bad Gateway` (with localized error keys) instead of silently returning `200 OK`.
- The `/api/overlay` endpoint now shares the ingest rate limiter (60 req/min/key) so an unauthenticated misconfiguration can no longer hammer it unbounded.
- Bumped Linux release-binary Go toolchain in `.github/workflows/release.yml` from `1.25.0` to `1.25.8`, matching the test jobs.
- GitLab CI `sign-dev` and `publish_dev_to_github` jobs upgraded from `alpine:3.20` to `alpine:3.23` to match the Dockerfile runtime base.
- All activity write paths (create, edit, delete, multi-plant create) now run inside database transactions. Partial failures no longer leave the activity log in a half-written state, and multi-plant creates pay one WAL fsync instead of N on SQLite.
- AC Infinity / EcoWitt scan handlers now route their sensor registration through the locked `findOrCreateSensor` helper, eliminating an unprotected SELECT-then-INSERT path that could create duplicate rows under concurrent scans.

### Deprecated

### Removed
- Removed the per-service `healthcheck:` blocks from the shipped
  `docker-compose.*.yml` files for the `isley` service. Compose-level
  healthchecks fully override the Dockerfile `HEALTHCHECK`, so the
  hardcoded `localhost:8080` probe ignored `ISLEY_PORT` even after the
  v0.1.42 Dockerfile fix. The Dockerfile healthcheck (which honors
  `${ISLEY_PORT:-8080}`) is now the single source of truth.
- Deleted the unused `jsonify` template helper (returned unescaped JSON; previous audits flagged it as a footgun) and the duplicate `formatDateISO` template func — `formatDate` already produced an identical ISO date.
- Removed two redundant placeholder/args slice loops in `getPlantsByStatus` whose results were immediately discarded by a re-declaration on the next line.
- Dropped the unused `ISLEY_MIGRATE_SQLITE` line from `docker-compose.migration.yml` — the variable was never read by Go code (the migration scan uses `ISLEY_DB_FILE`).

### Fixed
- Reopened #146: shipped Compose files were overriding the Dockerfile
  healthcheck and pinning the probe to port 8080. Users who changed
  `ISLEY_PORT` saw the container marked unhealthy. Resolved by removing
  the redundant Compose healthcheck blocks; existing deployments may
  need to `docker compose up -d --force-recreate` (or edit their local
  compose file) to pick up the new behavior.
- Sensor ingest endpoint no longer creates duplicate sensor rows under concurrent ingest of the same (source, device, type). Existing duplicates are merged on upgrade.

### Security
- Plant image uploads are now bounded with `http.MaxBytesReader` (250 MB per request, 50 MB per file) and rejected with `413 Request Entity Too Large` when exceeded — previously, files above the in-memory threshold spilled to disk unbounded.
- `LinkSensorsToPlant` now verifies every supplied sensor ID exists before persisting the JSON column, rejecting dangling references with `400` instead of silently storing them.
- Strain create/update endpoints now length-validate `name`, `description`, `short_desc`, and `new_breeder`, and reject `url` values that are not http/https — closes a long-standing input-validation gap on the strain form.
- Plant activity, measurement, and status edit endpoints now cap `note` at `MaxNotesLength` (5000) and reject malformed `date` values via the new `utils.ValidateDate` helper.
- Rate-limit logs no longer record raw `X-API-KEY` values; offending callers are now identified by a non-reversible 12-char SHA-256 prefix, preserving correlation while removing the credential from logs.
- `app.NewEngine` now requires `SessionSecret` to be at least 32 bytes (was: any non-empty length), preventing weak session keys.
- Trusted-proxy list now includes the IPv6 unique-local block (`fc00::/7`), so deployments that front Isley with an IPv6 reverse proxy see accurate `c.ClientIP()` values for rate limiting and audit logs.
- CSP-nonce generation now aborts the request with `500 Internal Server Error` on `crypto/rand.Read` failure instead of serving a page with a predictable empty nonce.
- Bumped bcrypt cost from 10 to 12 for password and API-key hashing. New env var `ISLEY_BCRYPT_COST` lets test suites lower the cost without weakening production defaults.
- Bounded watcher and scan response bodies with `io.LimitReader` (4 MiB cap) so a slow-drip server (notably user-supplied EcoWitt LAN addresses) cannot stream unbounded payloads inside the request timeout.
- Capped the `sensor_data_hourly` rollup queries with the same `MaxRawDataRows` limit the raw-data branches already used (defensive only).

## [0.1.42] - 2026-04-25

### Added
- Added #148 Activities list full view and export.  Activity log is now a top level item with searching / filtering /
export.  Additionally the plant details page includes an expandable "Notebook" view with activity details displayed.

### Changed
- Rebuilt the CI/CD pipeline around a CHANGELOG-driven release ceremony.
  Release notes now come from the curated `CHANGELOG.md` section matching
  `VERSION` rather than from parsed commit messages, which were structurally
  lossy after the GitLab → GitHub squash sync.
- GitHub workflow adds OCI image labels (revision, source, version, created)
  so published Docker Hub images are traceable back to the commit that built
  them.
- GitLab → GitHub squash-sync commit message now embeds the originating
  GitLab short SHA for cross-registry traceability.
- Tightened `VERSION` discipline: release workflow rejects a push to `main`
  if the tag already exists or the version string isn't strict SemVer.

### Deprecated

### Removed

### Fixed
- Fixed #146 Healthcheck didn't respect ISLEY_PORT

### Security
Bump golang to 1.25.9 for security updates
Bump modernc.org/sqlite to 1.49.1
Bump golang.org/x/text to 0.36.0
Bump golang.org/x/image to 0.39.0
Bump golang.org/x/crypto to 0.50.0

## 0.1.41b and earlier

Prior releases are recorded on
[GitHub Releases](https://github.com/dwot/isley/releases). Beginning with
`0.1.42` all user-visible changes are tracked here.
