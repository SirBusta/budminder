**Settings → Integrations** connects BudMinder to external hardware controllers, smart home platforms and public genetic catalogs. Each integration can be enabled or disabled independently.

## Sensors

- **AC Infinity**: authenticate cloud controller accounts, configure data polling frequency and toggle **Stay signed in**. See [AC Infinity](/manual/ac-infinity).
- **EcoWitt**: enable direct local gateway polling. Network IP addresses are designated during sensor scanning. See [Ecowitt](/manual/ecowitt).
- **Home Assistant**: configure your Home Assistant host address, port and long-lived access token. See [Home Assistant](/manual/home-assistant).

The global **Polling Interval** (configurable from 15 to 300 seconds, defaulting to 60 seconds) governs how frequently BudMinder polls all active hardware integrations for fresh telemetry.

## Strain lookups

- **StrainCompass Strain Import**: enables one-click cultivar imports on the Strains page and integrates StrainCompass lookup inside smoke report forms. Entering a **StrainCompass API key** is optional; basic searches work out of the box, while adding a key increases daily request rate limits.
- **CannaDB Strain Import**: allows BudMinder to supplement missing strain attributes from CannaDB and import seed packaging artwork. See [Adding a strain](/manual/adding-a-strain).
- **Breeder Suggestions**: provides auto-complete breeder suggestions as you type, complete with automated monthly synchronization. See [Breeders](/manual/breeders#breeder-suggestions).

Under **Advanced**, each lookup service includes a custom base URL field for redirecting queries to custom mirrors or local instances. Leave these fields blank for standard operation.

## Sensor API

Generate an **API Key** to authenticate custom microcontrollers or automated scripts, or toggle **Disable API ingest** to pause external incoming readings temporarily. See [Your own sensors](/manual/your-own-sensors).
