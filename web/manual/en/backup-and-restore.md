A backup is a single file with everything in BudMinder: plants, notebooks, strains, sensors, smoke reports and settings, plus, if you like, your photos and sensor history. Make one regularly, and keep a copy somewhere other than the BudMinder computer.

## Making a backup

Go to **Settings → Backup & data → Create Backup**.

1. Choose how much **Sensor History** to include: none, the last 7, 30 or 90 days, or all of it. Sensor history is usually the biggest part of a backup.
2. Tick **Include images** to add your photos and camera snapshots.
3. Click **Create Backup**.

The backup is saved on the server and appears under **Available Backups**. Click the download button to save a copy to your computer, or the trash can to delete one.

> **Tip:** Backups kept on the server are lost if that computer's drive fails. Download the ones you want to keep.

## Restoring a backup

> **Warning:** Restoring replaces everything in this BudMinder with the backup. Anything added since the backup was made is lost.

1. Go to **Settings → Backup & data → Restore Backup**.
2. Drag a BudMinder backup `.zip` file into the box, or click to choose it.
3. Click **Restore Backup** and wait until it finishes. Sensor polling pauses while it runs.

To move BudMinder to a new computer, make a backup on the old one, install BudMinder on the new one, and restore the backup there.

On SQLite, the database most people use, importing a lot of sensor history can be slow. Tick **Skip sensor data** to restore much faster without it.

## SQLite file transfer

Under **Advanced: SQLite File Transfer**, you can download the database file itself (**Download .db File**), or replace this BudMinder's database with one you upload (**Replace Database**). It's the quickest way to copy a whole SQLite setup. It doesn't include photos, which are kept in the uploads folder.

## Sensor history

**Data Retention** sets how many days of sensor readings to keep; older ones are removed once a day. **0** keeps everything forever.

## If you run BudMinder in Docker

Everything BudMinder stores is in its two folders (volumes): the data folder (`/app/data`), with the database and the backups, and the uploads folder (`/app/uploads`), with your photos and camera snapshots. Copying both, with BudMinder stopped, is a complete copy too.
