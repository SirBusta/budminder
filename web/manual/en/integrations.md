**Settings → Integrations** connects BudMinder to the services it reads from. Each one can be turned on or off on its own.

## Sensors

- **AC Infinity**: your AC Infinity account, the polling interval and **Stay signed in**. See [AC Infinity](/manual/ac-infinity).
- **EcoWitt**: turns Ecowitt reading on. The gateway's address is entered when you scan. See [Ecowitt](/manual/ecowitt).
- **Home Assistant**: its address and access token. See [Home Assistant](/manual/home-assistant).

The **Polling Interval** (15 to 300 seconds, 60 by default) sets how often BudMinder fetches new readings from all of them.

## Strain lookups

- **StrainCompass Strain Import** turns on **Find on StrainCompass** on the Strains page, and StrainCompass search in smoke reports. A **StrainCompass API key** is optional; StrainCompass works without one, and a key only raises its daily request limit.
- **CannaDB Strain Import** lets BudMinder fill in what StrainCompass doesn't have from CannaDB, and offer CannaDB's seed-pack pictures. See [Adding a strain](/manual/adding-a-strain).
- **Breeder Suggestions**: the list of breeder names suggested as you type, with a monthly refresh. See [Breeders](/manual/breeders#breeder-suggestions).

Under **Advanced**, each lookup service has an address field that can point BudMinder elsewhere. Leave these blank.

## Sensor API

The **API Key** for your own sensors and scripts, and **Disable API ingest** to block outside readings for a while. See [Your own sensors](/manual/your-own-sensors).
