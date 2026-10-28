Photos are linked directly to the date they were taken, displaying side by side with that day's notebook entry.

## Adding photos

Upload photos from any of these shortcuts:

- the camera icon next to **Add activity** in a plant's notebook
- **Add a photo** on any day that does not yet have photos
- **Add photos** on the Today page, or the camera button on a plant card
- the camera icon in a plant's row on the Plants list

Drag and drop photos directly into the upload window, or click to browse files on your device. You can upload multiple images at once. Verify the date for each image, add an optional description and click upload.

BudMinder automatically reads the capture date from image metadata (EXIF). If metadata is unavailable, it uses your currently selected calendar date (or today) and outlines the date field so you can verify it.

Your most recent upload becomes the featured hero image at the top of the plant's profile.

## Viewing photos

Click any photo to open the full-screen gallery viewer.

![The photo viewer: the date, the arrows, Decorate and Delete photo, and the thumbnails](fig:photo-viewer)

- {1} **Date and index:** shows the date the photo was taken and its index number within that day's gallery.
- {2} **Navigation:** click the left and right arrows to browse photos, or use the left and right arrow keys on your keyboard.
- {3} **Decorate:** adds custom text overlays and brand logos to a copy of the photo. See below.
- {4} **Delete photo:** permanently deletes the photo after confirmation. This action cannot be undone.
- {5} **Thumbnails:** quick-select gallery strip of all photos taken that day. Click any thumbnail to jump to it.

Close the photo viewer by clicking the X icon in the top right corner, or press the Esc key.

## Decorate

Decorate places custom text overlays on an exported copy of your photo, formatted for sharing on forums or social media. Your original high-resolution photo remains completely untouched.

- **Top Text and Bottom Text**: choose from automated tags (strain, breeder, plant name, day count or week number) or select **Custom** to enter your own text.
- **Placement**: select which corner of the image each text block appears in.
- **Select Font and Text Color**: customize the typography and color palette.
- **Select Logo**: overlays a custom watermark or logo uploaded under **Settings → Appearance**.

Click **Decorate** to generate your customized image. The preview opens in a window. To save it, right-click and choose **Save image**, or press and hold the image on a mobile device.

## Photo size and compression

Phone photos are often 3–5 MB each. Under **Settings → Photos**, BudMinder can compress plant photos to save storage space and make backups faster. Compression keeps the pixel dimensions, the orientation and the color profile; visible differences are generally imperceptible unless you zoom in significantly. Only JPEG plant photos are compressed. Smoke report photos, camera snapshots and PNG, GIF or WebP files are stored as uploaded.

- **Keep originals** (the default): photos are stored exactly as uploaded, without their hidden metadata.
- **Compression Options**: new plant photos are compressed at the selected **Quality** when they are added. Quality 75 is recommended; 90 keeps the most detail and 50 makes the smallest files.

A photo is only replaced when compressing it saves at least 10%. Older or already-compressed photos often save less, or would even grow, so BudMinder keeps the original. A photo that has been compressed is never compressed again.

### Choosing a quality

Click **Compare Compression Levels** to open the compare page in a new tab. It shows one of your plant photos (or a sample photo, before you have added any) next to a copy compressed the same way BudMinder would compress it.

- Press **Space** or click the green button to switch between the original and the compressed copy.
- Scroll to zoom, and hold and drag to move around the photo. **Fit**, **100%** and **300%** set the zoom directly.
- The quality list shows the file size at each quality. A note appears when a quality would save less than 10% or make the file larger.
- **Choose photo** picks another photo. Photos with fine detail, such as trichomes, leaf edges or a label, show the difference first.
- **Use quality NN** saves that quality and turns on compression for new photos.

### Compressing existing photos

Photos already in BudMinder only change when you use **Compress existing photos**. The **Existing photos** row shows how many plant photos have not been compressed yet, their current size and the estimated savings at the selected quality.

1. Select the quality, then click **Compress existing photos**.
1. Review the summary. **Back up original photos before compressing (Recommended)** is selected by default and needs about as much free disk space as the photos use now. This copy covers plant photos only; for a complete backup of BudMinder, use **Settings → Backup & data** (see [Backup and restore](backup-and-restore.md)).
1. Click **Compress … photos**. Photos are processed one at a time in the background, so you can keep using BudMinder. A line under the top menu shows the progress on every page.

> **Warning:** Compressed photos overwrite the originals. Without the backup, the original files cannot be recovered.

Click **Stop** to pause the run after the current photo. If BudMinder is closed during a run, the run is paused as well, and the line under the top menu turns amber. Click **Resume** to continue where it stopped, or **Finish Without Remaining Photos** to end the run and leave the remaining photos as they are. Photos already compressed are not affected either way. The photo settings cannot be changed while a run is running or paused.

### The backup folder

When the run finishes, Settings → Photos shows how many photos were compressed, the size of the backup and the folder the originals were copied to: `data/backups/photos-before-compression-<date>` inside the BudMinder folder. Click **Copy** to copy the path; when BudMinder runs as the downloadable program on the same computer, **Open folder** opens it. The panel stays on Settings → Photos while the backup is kept, so you can look through your plant photos and come back to it.

- **Restore original photos** puts the originals back in place of the compressed photos and deletes the backup folder. Photos deleted since the run stay deleted. Restored photos count as not compressed, so a later run can compress them again.
- **Delete backup now** keeps the compressed photos and deletes the folder to free up the space.

BudMinder deletes the folder automatically 7 days after the run finished; after that the originals can no longer be restored.

In Docker, the folder is in the data volume; on Unraid that is `/mnt/user/appdata/budminder/data/backups/` by default.
