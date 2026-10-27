A BudMinder backup consolidates your complete cultivation environment into a single portable archive: plant profiles, activity notebooks, cultivar libraries, sensor mappings, smoke reports and system settings, plus optional high-resolution photos and historical climate logs. Create backups regularly and store copies off the host server to ensure data safety.

## Making a backup

Navigate to **Settings → Backup & data → Create Backup**.

1. Select how much **Sensor History** to include: None, 7 days, 30 days, 90 days or Full History. Historical sensor telemetry typically makes up the largest portion of archive size.
1. Check **Include images** to package your plant photo galleries and camera snapshots.
1. Click **Create Backup**.

BudMinder generates the archive locally and lists it under **Available Backups**. Click the download icon to save a copy to your local machine, or click the trash can icon to delete older snapshots.

> **Tip:** Backups stored solely on the host server will be lost if that drive fails. Always download important backups to external or cloud storage.

## Restoring a backup

> **Warning:** Restoring an archive overwrites your current BudMinder database entirely. Any entries or modifications logged after the backup was generated will be permanently replaced.

1. Go to **Settings → Backup & data → Restore Backup**.
1. Drag and drop a BudMinder backup `.zip` archive into the upload target, or click to select the file from your computer.
1. Click **Restore Backup** and allow the process to finish. Active sensor polling pauses automatically during the restore.

To migrate BudMinder to a new computer or server, generate a backup on the original machine, install BudMinder on the new hardware and restore the archive there.

If you don't need the sensor history, check **Skip sensor data** to restore everything else.

## Photo backups before compression

**Compress existing photos** (Settings → Photos) can copy the original photos to a separate folder, `data/backups/photos-before-compression-<date>`, before it compresses them. That folder is not a BudMinder backup: it is not listed under **Available Backups**, cannot be restored from this page, and is deleted automatically 7 days after the run finished. See [Photos](/manual/photos#photo-size-and-compression).

## SQLite file transfer

Under **Advanced: SQLite File Transfer**, you can download the raw database file directly (**Download .db File**) or swap out the active database by uploading an existing file (**Replace Database**). This provides a fast method for duplicating an entire SQLite installation. Note that raw database transfers do not include media files, which reside separately in the server's uploads folder.

## Sensor history

The **Data Retention** setting specifies how many days of sensor telemetry to retain. Older readings are automatically purged once daily in the background. Setting this value to **0** preserves all historical climate logs indefinitely.

## If you run BudMinder in Docker

When deployed via Docker, all persistent application data lives inside two mapped directory volumes: the data volume (`/app/data`) containing the database and backup archives, and the uploads volume (`/app/uploads`) containing plant photos and camera captures. Creating a direct copy of both folders while the BudMinder container is stopped produces a full manual system clone.
