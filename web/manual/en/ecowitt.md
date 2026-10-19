BudMinder communicates directly with Ecowitt local gateways (including GW1000, GW1100 and GW2000 models) across your local network. No Ecowitt cloud accounts, external servers or internet access are required.

## Connecting

1. Go to **Settings → Integrations → EcoWitt** and toggle on **Enable EcoWitt Sensor Monitoring**.
1. Navigate to [Manage sensors](/manual/manage-sensors) and click **Scan Ecowitt**.
1. Enter the gateway's local IP address (for example, *192.168.1.40*) and run the scan.
1. Assign each discovered sensor (such as multi-channel temperature/humidity probes or soil moisture sensors) to a tent zone and click **Add**.

BudMinder polls the gateway automatically based on the global **Polling Interval** defined in Settings.

> **Tip:** Assign a static IP address or DHCP reservation to your Ecowitt gateway in your router settings. If the router reassigns a new IP address after a reboot, BudMinder will lose connection to the device.

## If readings stop

The Ecowitt connection card under Manage sensors will report **Not responding** if communication drops. Confirm the gateway is powered on, connected to your Wi-Fi network and that its local IP address has not changed.
