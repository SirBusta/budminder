Each tent can have a camera. BudMinder shows its latest snapshot on the Today page, and its live picture on the Today page and on the pages of the plants in that tent.

## Adding a camera

1. Go to **Settings → Cameras → Streams** and click **Add Stream**.
2. Give it a **Stream Name**, choose the tent under **Stream Zone**, and paste the camera's **Stream URL**.
3. Leave **Visibility** on **Show**. **Hide** keeps the camera without showing it anywhere.

## Which addresses work

BudMinder works with web (`http://` or `https://`) addresses:

- **A snapshot address**: one that returns a single picture. Many cameras have one.
- **An MJPEG stream**: a continuous stream of pictures. BudMinder takes the first one as its snapshot.
- **An HLS stream**: an address ending in `.m3u8`. Only this kind plays live in BudMinder (the **Live** button). Its snapshot comes from the streaming software's own thumbnail, such as Owncast's.

`rtsp://` addresses, which many security cameras use, don't work directly. A free restreaming tool such as go2rtc or Frigate can turn them into an HLS stream and a snapshot address that BudMinder can use.

## Snapshots

In **Settings → Cameras → Snapshots**, turn on **Enable Stream Grab** and set **Stream Grab Interval**, from 5 to 60 minutes. BudMinder grabs a still from each camera on that schedule and uses it as the camera's preview. Only the newest snapshot is kept.

## Watching live

- On the Today page, click **Live** on a tent's camera.
- On a plant's page, switch the picture at the top right from **Photo** to **Live**.

Both need an HLS (`.m3u8`) address.
