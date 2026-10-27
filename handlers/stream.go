package handlers

import (
	"database/sql"
	"fmt"
	"isley/logger"
	"isley/model/types"
	"isley/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func GetStreams(db *sql.DB) []types.Stream {
	streams := []types.Stream{}
	fieldLogger := logger.Log.WithField("func", "GetStreams")
	rows, err := db.Query("SELECT s.id, s.name, url, zone_id, visible, z.name as zone_name FROM streams s left outer join zones z on s.zone_id = z.id")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read stream")
		return streams
	}
	defer rows.Close()

	stream := types.Stream{}
	for rows.Next() {
		var id, zoneID uint
		var visible bool
		var name, url, zoneName string
		err = rows.Scan(&id, &name, &url, &zoneID, &visible, &zoneName)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read stream")
			continue
		}
		stream = types.Stream{ID: id, Name: name, URL: url, ZoneID: zoneID, ZoneName: zoneName, Visible: visible}
		streams = append(streams, stream)
	}

	return streams
}

func AddStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddStreamHandler")
	var stream struct {
		Name    string `json:"stream_name"`
		URL     string `json:"url"`
		ZoneID  int    `json:"zone_id"`
		Visible bool   `json:"visible"`
	}
	if err := c.ShouldBindJSON(&stream); err != nil {
		fieldLogger.WithError(err).Error("Failed to add stream")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("stream_name", stream.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStreamURL(stream.URL); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Add stream to database
	db := DBFromContext(c)

	// Insert new stream and return new id
	var id int
	err := db.QueryRow("INSERT INTO streams (name, url, zone_id, visible) VALUES ($1, $2, $3, $4) RETURNING id", stream.Name, stream.URL, stream.ZoneID, stream.Visible).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add stream")
		apiInternalError(c, "api_failed_to_add_stream")
		return
	}

	streams := GetStreams(db)
	ConfigStoreFromContext(c).SetStreams(streams)

	latestFileName := fmt.Sprintf("stream_%d_latest%s", id, filepath.Ext(".jpg"))
	latestSavePath := filepath.Join(StreamDirFromContext(c), latestFileName)
	utils.GrabWebcamImage(stream.URL, latestSavePath)

	c.JSON(http.StatusCreated, gin.H{"id": id, "streams": streams})
}

func UpdateStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateStreamHandler")
	id := c.Param("id")
	var stream struct {
		Name    string `json:"stream_name"`
		URL     string `json:"url"`
		ZoneID  int    `json:"zone_id"`
		Visible bool   `json:"visible"`
	}
	if err := c.ShouldBindJSON(&stream); err != nil {
		fieldLogger.WithError(err).Error("Failed to update stream")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("stream_name", stream.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStreamURL(stream.URL); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Update stream in database
	db := DBFromContext(c)

	// visible is a BOOLEAN column: pass the bool itself (an int is refused by Postgres)
	_, err := db.Exec("UPDATE streams SET name = $1, url = $2, zone_id = $3, visible = $4 WHERE id = $5", stream.Name, stream.URL, stream.ZoneID, stream.Visible, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update stream")
		apiInternalError(c, "api_failed_to_update_stream")
		return
	}

	streams := GetStreams(db)
	ConfigStoreFromContext(c).SetStreams(streams)

	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_stream_updated"), "streams": streams})
}

func DeleteStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteStreamHandler")
	id := c.Param("id")

	// Delete stream from database
	db := DBFromContext(c)

	// Delete stream from database
	_, err := db.Exec("DELETE FROM streams WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete stream")
		apiInternalError(c, "api_failed_to_delete_stream")
		return
	}

	streams := GetStreams(db)
	ConfigStoreFromContext(c).SetStreams(streams)

	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_stream_deleted"), "streams": streams})
}

func GetStreamsByZoneHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetStreamsByZoneHandler")
	db := DBFromContext(c)

	rows, err := db.Query("SELECT s.id, s.name, s.url, z.name as zone_name, visible FROM streams s left outer join zones z on s.zone_id = z.id")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read streams")
		apiInternalError(c, "api_failed_to_read_streams")
		return
	}
	defer rows.Close()

	// Guests (guest mode) see only visible streams, and no addresses:
	// a camera address can carry its password.
	guest := sessions.Default(c).Get("logged_in") != true
	streamsByZone := make(map[string][]types.Stream)
	for rows.Next() {
		var id int
		var name, url, zoneName string
		var visible bool
		err = rows.Scan(&id, &name, &url, &zoneName, &visible)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read streams")
			apiInternalError(c, "api_failed_to_read_streams")
			return
		}
		if guest {
			if !visible {
				continue
			}
			url = ""
		}
		streamsByZone[zoneName] = append(streamsByZone[zoneName], types.Stream{ID: uint(id), Name: name, URL: url, ZoneName: zoneName, Visible: visible})
	}

	c.JSON(http.StatusOK, streamsByZone)
}

func DeleteStreamByID(db *sql.DB, id string) error {
	fieldLogger := logger.Log.WithField("func", "DeleteStreamByID")

	_, err := db.Exec("DELETE FROM streams WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Error deleting sensor")
		return err
	}
	return nil
}

// StreamSnapshotURL is where the page loads a camera's latest snapshot.
func StreamSnapshotURL(id int) string { return fmt.Sprintf("/streams/%d/snapshot", id) }

// StreamSnapshotHandler serves a camera's latest snapshot. /uploads is
// public and the file name is easy to guess, so snapshots are served here
// instead: signed in, any camera; a guest (guest mode), only cameras
// marked visible.
// GET /streams/:id/snapshot
func StreamSnapshotHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.Status(http.StatusNotFound)
		return
	}
	var visible bool
	if err := DBFromContext(c).QueryRow("SELECT visible FROM streams WHERE id = $1", id).Scan(&visible); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if !visible && sessions.Default(c).Get("logged_in") != true {
		c.Status(http.StatusNotFound)
		return
	}
	path := filepath.Join(StreamDirFromContext(c), fmt.Sprintf("stream_%d_latest.jpg", id))
	if _, err := os.Stat(path); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.File(path)
}
