BudMinder has no window of its own. It runs in the background and you use it in your web browser, so **closing the browser doesn't stop BudMinder**. It keeps running for your phone and your sensors. While BudMinder is stopped, sensors aren't recorded.

## Windows

**Starting.** Double-click the **BudMinder** icon on your desktop or in the Start menu. It starts BudMinder in the background if it isn't running yet, then opens it in your browser.

**The icon by the clock.** While BudMinder runs, its leaf icon sits in the taskbar next to the clock. Click it to open BudMinder. Right-click it for the menu:

- **Open BudMinder** opens it in your browser.
- **Open on your phone…** shows a code to scan with your phone's camera. See [My phone can't open BudMinder](/manual/troubleshooting#my-phone-can-t-open-budminder-windows) if the phone won't connect.
- **Copy phone address** copies the address a phone on your Wi-Fi uses.
- **Update to …** appears when a new version is out. The icon also gets an amber frame.
- **Open BudMinder folder** shows your data, photos and backups.
- **Start with Windows** starts BudMinder in the background each time you sign in to Windows, so sensors keep recording. Click it again to turn it off.
- **Stop BudMinder** stops it.

> **Tip:** Don't see the icon? Windows tucks new icons away. Click the **^** arrow next to the clock and drag the BudMinder icon onto the taskbar, so it stays in view.

**Stopping.** Right-click the icon by the clock and choose **Stop BudMinder**, or open the Start menu and click **Stop BudMinder**. Closing BudMinder's small welcome window doesn't stop it: the window goes into the icon by the clock.

## Linux

**Starting.** Double-click the **BudMinder** icon on your desktop or in the app menu. It starts BudMinder in the background if needed and opens it in your browser.

**Stopping.** Right-click the BudMinder icon and choose **Stop BudMinder**. If you started it from a terminal, press **Ctrl+C** there or close the terminal. If you set it up to start with the computer (`systemctl --user`), stop it with `systemctl --user stop budminder`.

## macOS (beta)

BudMinder runs in a Terminal window. Close that window, or press **Ctrl+C** in it, to stop BudMinder.

## Docker and Unraid

BudMinder runs as a container and starts by itself with Docker, so you normally never stop it.

- **Unraid:** on the **Docker** tab, click the BudMinder icon and choose **Stop**, **Start** or **Restart**.
- **Docker Compose:** in the folder with your `docker-compose.yml`, run `docker compose stop` to stop it and `docker compose up -d` to start it again.

Your data stays in the container's volumes either way.
