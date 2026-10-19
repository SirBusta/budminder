If your sensors are already in Home Assistant, BudMinder can read them: Govee, SwitchBot, Inkbird, Zigbee, ESPHome and anything else Home Assistant shows as a number, such as temperature, humidity, CO₂, soil moisture, light or EC. BudMinder only reads values. It never changes anything in Home Assistant.

## Connecting

1. In Home Assistant, create a separate user for BudMinder (**Settings → People → Add person**, allow login, not an administrator).
2. Log in to Home Assistant as that user, open your profile → **Security** → **Long-lived access tokens**, and create a token. Copy it; Home Assistant shows it only once.
3. In BudMinder, go to **Settings → Integrations → Home Assistant** and turn on **Read sensors from Home Assistant**.
4. Enter Home Assistant's **Address** with its port, for example *http://192.168.1.20:8123*, and paste the **Access token**.
5. Click **Test connection**. BudMinder says how many sensors it found, and how many of them are useful for growing.
6. Go to [Manage sensors](/manual/manage-sensors) and click **Scan Home Assistant**. Add the sensors you want and put each in a tent.

Once saved, the token isn't shown again.

## Choosing sensors

The scan lists the sensors useful for growing first, with their current readings. Type in **Filter found sensors** to find one by name. Batteries, power meters and similar sensors are hidden until you turn on **Show all**. **Add these** adds every sensor in the list you're looking at.

## If readings stop

If Home Assistant restarts or is offline for a while, BudMinder picks the readings back up when it returns. The Home Assistant card on Manage sensors shows **Not responding** while it can't reach it, or **Token refused** if the token was deleted or mistyped. Make a new token and paste it in Settings.
