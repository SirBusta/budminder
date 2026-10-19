BudMinder reads Ecowitt gateways (such as the GW1000, GW1100 and GW2000) directly on your own network. No Ecowitt account or cloud is involved.

## Connecting

1. Go to **Settings → Integrations → EcoWitt** and turn on **Enable EcoWitt Sensor Monitoring**.
2. Go to [Manage sensors](/manual/manage-sensors) and click **Scan Ecowitt**.
3. Enter the gateway's address on your network, for example *192.168.1.40*, and scan.
4. Put each sensor found, such as a temperature and humidity sensor or a soil moisture probe, in a tent and click **Add**.

BudMinder then reads the gateway on the same schedule as the other sensors (the **Polling Interval** in Settings).

> **Tip:** Give the gateway a fixed address in your router, so it doesn't change after a restart. If the address changes, BudMinder can no longer reach the gateway.

## If readings stop

The Ecowitt card on Manage sensors shows **Not responding** when BudMinder can't reach the gateway. Check that the gateway is powered on and connected, and that its address hasn't changed.
