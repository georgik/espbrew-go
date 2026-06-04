package persistence

import bolt "go.etcd.io/bbolt"

const (
	// Bucket names
	bucketDevices        = "devices"
	bucketJobs           = "jobs"
	bucketHistory        = "history"
	bucketMeta           = "meta"
	bucketBoundingBoxes  = "bounding_boxes"
	bucketCalibrations   = "calibrations"
	bucketFlashHashes    = "flash_hashes"
	bucketCameraSettings = "camera_settings"

	// Key prefixes
	prefixDevice        = "device:"
	prefixIndexMAC      = "index:mac:"
	prefixIndexAlias    = "index:alias:"
	prefixJob           = "job:"
	prefixIndexPending  = "index:pending:"
	prefixIndexJobDev   = "index:device:"
	prefixHistDevice    = "hist:device:"
	prefixHistDate      = "hist:"
	prefixBoundingBox   = "bbox:"
	prefixCalibration   = "calib:"
	prefixCamIndex      = "index:cam:"
	prefixDevIndex      = "index:dev:"
	prefixCalibCamIndex = "index:calib:cam:"

	// Meta keys
	metaSchemaVersion = "schema_version"
	metaNextManualID  = "next_manual_id"

	// Current schema version
	currentSchemaVersion = 1
)

func initBuckets(tx *bolt.Tx) error {
	buckets := [][]byte{
		[]byte(bucketDevices),
		[]byte(bucketJobs),
		[]byte(bucketHistory),
		[]byte(bucketMeta),
		[]byte(bucketBoundingBoxes),
		[]byte(bucketCalibrations),
		[]byte(bucketFlashHashes),
		[]byte(bucketCameraSettings),
	}

	for _, name := range buckets {
		_, err := tx.CreateBucketIfNotExists(name)
		if err != nil {
			return err
		}
	}

	meta := tx.Bucket([]byte(bucketMeta))
	if v := meta.Get([]byte(metaSchemaVersion)); v == nil {
		if err := meta.Put([]byte(metaSchemaVersion), itob(currentSchemaVersion)); err != nil {
			return err
		}
	}

	return nil
}

func deviceKey(deviceID string) []byte {
	return []byte(prefixDevice + deviceID)
}

func macIndexKey(mac string) []byte {
	return []byte(prefixIndexMAC + mac)
}

func aliasIndexKey(alias string) []byte {
	return []byte(prefixIndexAlias + alias)
}

func jobKey(jobID string) []byte {
	return []byte(prefixJob + jobID)
}

func jobDeviceIndexKey(devicePath string) []byte {
	return []byte(prefixIndexJobDev + devicePath)
}

func historyDeviceKey(deviceID, yearMonth string) []byte {
	return []byte(prefixHistDevice + deviceID + ":" + yearMonth)
}

func historyDateKey(date string) []byte {
	return []byte(prefixHistDate + date)
}

func itob(v int) []byte {
	b := make([]byte, 8)
	for i := uint64(0); i < 8; i++ {
		b[i] = byte(v >> (i * 8))
	}
	return b
}

func btoi(b []byte) int {
	var v int
	for i := 0; i < 8; i++ {
		if i < len(b) {
			v |= int(b[i]) << (i * 8)
		}
	}
	return v
}

// Bounding box key helpers
func boundingBoxKey(id string) []byte {
	return []byte(prefixBoundingBox + id)
}

func cameraIndexKey(cameraID, bboxID string) []byte {
	return []byte(prefixCamIndex + cameraID + ":" + bboxID)
}

func cameraIndexPrefix(cameraID string) []byte {
	return []byte(prefixCamIndex + cameraID + ":")
}

func deviceIndexKey(deviceID, bboxID string) []byte {
	return []byte(prefixDevIndex + deviceID + ":" + bboxID)
}

func deviceIndexPrefix(deviceID string) []byte {
	return []byte(prefixDevIndex + deviceID + ":")
}

// Calibration key helpers
func calibrationKey(id string) []byte {
	return []byte(prefixCalibration + id)
}

func calibrationCameraIndexKey(cameraID string) []byte {
	return []byte(prefixCalibCamIndex + cameraID)
}
