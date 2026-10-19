Common problems and what to do about them. If yours isn't here, the logs (below) usually say what went wrong.

## I forgot my password

BudMinder has no password reset. If you're still logged in on another browser or phone (for example with **Remember Me**), go to **Settings → General → Password** there and set a new one.

## AC Infinity readings stopped

The Today page says **AC Infinity refused BudMinder's sign-in**. Go to **Settings → Integrations → AC Infinity**, click **Retrieve Token** and sign in again. Tick **Stay signed in** so BudMinder signs back in by itself next time. See [AC Infinity](/manual/ac-infinity#stay-signed-in).

## Ecowitt or Home Assistant readings stopped

Check the card on [Manage sensors](/manual/manage-sensors). **Not responding** means BudMinder can't reach it: check it's switched on and its address hasn't changed. **Token refused** (Home Assistant) means the access token was deleted; make a new one and paste it in Settings.

## A sensor says when its last reading was

The sensor stopped sending readings. Check the device itself (power, batteries, Wi-Fi), and its source on Manage sensors.

## Dates or times are off

Set your **Timezone** in **Settings → General**. Without it, BudMinder uses the server's own timezone, which on a home server or in Docker is often UTC.

## Guest view didn't change

Turning guest view on or off takes effect after BudMinder restarts.

## There's no Live button for my camera

Only cameras with an HLS address (ending in `.m3u8`) play live. See [Tent cameras](/manual/tent-cameras#which-addresses-work).

## A strain lookup doesn't work

Check that **StrainCompass Strain Import** is on in **Settings → Integrations**, and that the BudMinder computer can reach the internet. StrainCompass also limits how many searches can be made each day without an API key.

## The harvest estimate says Not set

The strain has no flowering time, or (for a photoperiod plant) the plant hasn't entered Flower yet. Add the flowering time on the strain's page with **Edit**. See [Plant page](/manual/plant-page#growth-stages).

## The logs

**Settings → Logs** shows what BudMinder has been doing.

- **App Log** shows what BudMinder did; **Access Log** shows which pages were opened.
- Choose **How many lines**, show only **Warnings** or **Errors**, or search the log.
- **Download** saves the log, for example to share when asking for help.

**Log Level**, under **Log detail**, sets how much is written. **Info** is right for everyday use. Switch to **Debug** while chasing a problem, then back again; the change takes effect straight away.
