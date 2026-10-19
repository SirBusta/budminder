Every tent can integrate a dedicated camera. BudMinder displays periodic still previews on the Today dashboard and streams real-time video across the Today page and individual plant profiles.

## Adding a camera

1. Go to **Settings → Cameras → Streams** and click **Add Stream**.
1. Enter a descriptive **Stream Name**, select the target tent under **Stream Zone**, and paste the camera's **Stream URL**.
1. Ensure **Visibility** is set to **Show**. Setting visibility to **Hide** retains stream settings while suppressing display across dashboards.

## Which addresses work

BudMinder supports standard HTTP/HTTPS web endpoints:

- **Snapshot endpoints**: URLs returning a static JPEG/PNG image on request. Most IP cameras provide a direct snapshot URL.
- **MJPEG streams**: continuous video streams delivered as multipart JPEG frames. BudMinder captures the initial frame for dashboard previews.
- **HLS streams**: stream addresses ending in `.m3u8`. Only HLS endpoints support interactive in-browser live streaming via the **Live** button. Preview thumbnails are pulled automatically from server stream metadata (such as Owncast).

Native `rtsp://` feeds common to security cameras cannot be rendered directly in web browsers. Use a local restreaming bridge (such as go2rtc or Frigate) to convert RTSP feeds into browser-compatible HLS and snapshot URLs.

## Snapshots

Under **Settings → Cameras → Snapshots**, toggle on **Enable Stream Grab** and configure the **Stream Grab Interval** (adjustable from 5 to 60 minutes). BudMinder captures a still frame from each camera on that schedule to serve as dashboard preview tiles, keeping only the most recent capture.

## Watching live

- On the Today dashboard, click **Live** on any tent camera card.
- On a plant profile, toggle the media preview in the upper right from **Photo** to **Live**.

Both live streaming interfaces require a valid HLS (`.m3u8`) stream URL.
