package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempDB(t *testing.T) string {
	return filepath.Join(t.TempDir(), "test.db")
}

func TestOpen(t *testing.T) {
	tmpdb := tempDB(t)

	store, err := Open(DefaultConfig(tmpdb))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if store.db == nil {
		t.Error("db is nil")
	}
}

func TestOpenInvalidPath(t *testing.T) {
	_, err := Open(DefaultConfig("/invalid/path/that/cannot/be/created/test.db"))
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestSaveDevice(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dev := &DeviceRecord{
		DeviceID:   "esp-aa:bb:cc:dd:ee:ff",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
		ChipRev:    "1.0",
		FlashSize:  8 * 1024 * 1024,
		PSRAMSize:  8 * 1024 * 1024,
		BoardModel: "ESP32-S3-DevKitC-1",
		Tags:       []string{"dev", "test"},
	}

	if err := store.SaveDevice(dev); err != nil {
		t.Fatalf("SaveDevice: %v", err)
	}

	if dev.FirstSeen.IsZero() {
		t.Error("FirstSeen not set")
	}
	if dev.LastSeen.IsZero() {
		t.Error("LastSeen not set")
	}
}

func TestSaveDeviceNil(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	err = store.SaveDevice(nil)
	if err == nil {
		t.Error("expected error for nil device")
	}
}

func TestSaveDeviceNoID(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dev := &DeviceRecord{
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
	}

	err = store.SaveDevice(dev)
	if err == nil {
		t.Error("expected error for missing device_id")
	}
}

func TestGetDevice(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	original := &DeviceRecord{
		DeviceID:   "esp-test-1",
		MACAddress: "11:22:33:44:55:66",
		ChipType:   "ESP32",
		Tags:       []string{"tag1"},
	}

	if err := store.SaveDevice(original); err != nil {
		t.Fatal(err)
	}

	dev, err := store.GetDevice("esp-test-1")
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}

	if dev.DeviceID != "esp-test-1" {
		t.Errorf("DeviceID = %s, want esp-test-1", dev.DeviceID)
	}
	if dev.ChipType != "ESP32" {
		t.Errorf("ChipType = %s, want ESP32", dev.ChipType)
	}
	if len(dev.Tags) != 1 || dev.Tags[0] != "tag1" {
		t.Errorf("Tags = %v, want [tag1]", dev.Tags)
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.GetDevice("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent device")
	}
}

func TestGetDeviceByMAC(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dev := &DeviceRecord{
		DeviceID:   "esp-mac-test",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
	}

	if err := store.SaveDevice(dev); err != nil {
		t.Fatal(err)
	}

	found, err := store.GetDeviceByMAC("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("GetDeviceByMAC: %v", err)
	}

	if found.DeviceID != "esp-mac-test" {
		t.Errorf("DeviceID = %s, want esp-mac-test", found.DeviceID)
	}
}

func TestGetDeviceByMACNotFound(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.GetDeviceByMAC("00:00:00:00:00:00")
	if err == nil {
		t.Error("expected error for nonexistent MAC")
	}
}

func TestGetDeviceByAlias(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dev := &DeviceRecord{
		DeviceID: "esp-alias-test",
		ChipType: "ESP32",
		Aliases:  []string{"devkit1", "test-device"},
	}

	if err := store.SaveDevice(dev); err != nil {
		t.Fatal(err)
	}

	found, err := store.GetDeviceByAlias("devkit1")
	if err != nil {
		t.Fatalf("GetDeviceByAlias: %v", err)
	}

	if found.DeviceID != "esp-alias-test" {
		t.Errorf("DeviceID = %s, want esp-alias-test", found.DeviceID)
	}
}

func TestMACConflict(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dev1 := &DeviceRecord{
		DeviceID:   "esp-1",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32",
	}

	dev2 := &DeviceRecord{
		DeviceID:   "esp-2",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
	}

	if err := store.SaveDevice(dev1); err != nil {
		t.Fatal(err)
	}

	err = store.SaveDevice(dev2)
	if err == nil {
		t.Error("expected error for MAC conflict")
	}
}

func TestUpdateDevice(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	original := &DeviceRecord{
		DeviceID:   "esp-update-test",
		MACAddress: "11:22:33:44:55:66",
		ChipType:   "ESP32",
		Tags:       []string{"old"},
	}

	if err := store.SaveDevice(original); err != nil {
		t.Fatal(err)
	}

	updated := &DeviceRecord{
		DeviceID:   "esp-update-test",
		MACAddress: "11:22:33:44:55:66",
		ChipType:   "ESP32-S3",
		Tags:       []string{"new"},
	}

	if err := store.SaveDevice(updated); err != nil {
		t.Fatal(err)
	}

	dev, err := store.GetDevice("esp-update-test")
	if err != nil {
		t.Fatal(err)
	}

	if dev.ChipType != "ESP32-S3" {
		t.Errorf("ChipType = %s, want ESP32-S3", dev.ChipType)
	}
	if len(dev.Tags) != 1 || dev.Tags[0] != "new" {
		t.Errorf("Tags = %v, want [new]", dev.Tags)
	}
}

func TestListDevices(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	devices := []*DeviceRecord{
		{DeviceID: "esp-1", MACAddress: "11:11:11:11:11:11", ChipType: "ESP32"},
		{DeviceID: "esp-2", MACAddress: "22:22:22:22:22:22", ChipType: "ESP32-S3"},
		{DeviceID: "esp-3", MACAddress: "33:33:33:33:33:33", ChipType: "ESP32-C3"},
	}

	for _, dev := range devices {
		if err := store.SaveDevice(dev); err != nil {
			t.Fatal(err)
		}
	}

	list, err := store.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListDevices returned %d devices, want 3", len(list))
	}
}

func TestListDevicesEmpty(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	list, err := store.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}

	if len(list) != 0 {
		t.Errorf("ListDevices returned %d devices, want 0", len(list))
	}
}

func TestDeleteDevice(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dev := &DeviceRecord{
		DeviceID:   "esp-delete-test",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32",
		Aliases:    []string{"to-delete"},
	}

	if err := store.SaveDevice(dev); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteDevice("esp-delete-test"); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}

	_, err = store.GetDevice("esp-delete-test")
	if err == nil {
		t.Error("expected error after delete")
	}

	_, err = store.GetDeviceByMAC("aa:bb:cc:dd:ee:ff")
	if err == nil {
		t.Error("MAC index not cleaned up")
	}

	_, err = store.GetDeviceByAlias("to-delete")
	if err == nil {
		t.Error("Alias index not cleaned up")
	}
}

func TestDeleteDeviceNotFound(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	err = store.DeleteDevice("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent device")
	}
}

func TestGenerateManualID(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	id1, err := store.GenerateManualID("ESP32-S3")
	if err != nil {
		t.Fatalf("GenerateManualID: %v", err)
	}

	id2, err := store.GenerateManualID("ESP32-S3")
	if err != nil {
		t.Fatalf("GenerateManualID: %v", err)
	}

	if id1 == id2 {
		t.Errorf("Generated same ID twice: %s", id1)
	}
}

func TestSaveJob(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	job := &JobRecord{
		ID:           "job-1",
		FirmwarePath: "/path/to/firmware.bin",
		DevicePath:   "/dev/cu.usbserial",
		Status:       JobStatusPending,
		Metadata:     map[string]string{"source": "api"},
	}

	if err := store.SaveJob(job); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}

	if job.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}
}

func TestGetJob(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	original := &JobRecord{
		ID:           "job-get-test",
		FirmwarePath: "/firmware.bin",
		DevicePath:   "/dev/ttyUSB0",
		Status:       JobStatusPending,
	}

	if err := store.SaveJob(original); err != nil {
		t.Fatal(err)
	}

	job, err := store.GetJob("job-get-test")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	if job.ID != "job-get-test" {
		t.Errorf("ID = %s, want job-get-test", job.ID)
	}
	if job.FirmwarePath != "/firmware.bin" {
		t.Errorf("FirmwarePath = %s, want /firmware.bin", job.FirmwarePath)
	}
}

func TestListPendingJobs(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	jobs := []*JobRecord{
		{ID: "job-1", DevicePath: "/dev/1", Status: JobStatusPending},
		{ID: "job-2", DevicePath: "/dev/2", Status: JobStatusRunning},
		{ID: "job-3", DevicePath: "/dev/3", Status: JobStatusPending},
	}

	for _, job := range jobs {
		if err := store.SaveJob(job); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := store.ListPendingJobs()
	if err != nil {
		t.Fatalf("ListPendingJobs: %v", err)
	}

	if len(pending) != 2 {
		t.Errorf("ListPendingJobs returned %d jobs, want 2", len(pending))
	}
}

func TestListJobsByDevice(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	jobs := []*JobRecord{
		{ID: "job-1", DevicePath: "/dev/ttyUSB0", Status: JobStatusCompleted},
		{ID: "job-2", DevicePath: "/dev/ttyUSB0", Status: JobStatusPending},
		{ID: "job-3", DevicePath: "/dev/ttyUSB1", Status: JobStatusPending},
	}

	for _, job := range jobs {
		if err := store.SaveJob(job); err != nil {
			t.Fatal(err)
		}
	}

	deviceJobs, err := store.ListJobsByDevice("/dev/ttyUSB0")
	if err != nil {
		t.Fatalf("ListJobsByDevice: %v", err)
	}

	if len(deviceJobs) != 2 {
		t.Errorf("ListJobsByDevice returned %d jobs, want 2", len(deviceJobs))
	}
}

func TestJobStatusTransition(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	job := &JobRecord{
		ID:         "job-status",
		DevicePath: "/dev/test",
		Status:     JobStatusPending,
	}

	if err := store.SaveJob(job); err != nil {
		t.Fatal(err)
	}

	pending, _ := store.ListPendingJobs()
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending job, got %d", len(pending))
	}

	job.Status = JobStatusRunning
	if err := store.SaveJob(job); err != nil {
		t.Fatal(err)
	}

	pending, _ = store.ListPendingJobs()
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending jobs after status change, got %d", len(pending))
	}
}

func TestDeleteJob(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	job := &JobRecord{
		ID:         "job-delete",
		DevicePath: "/dev/test",
		Status:     JobStatusPending,
	}

	if err := store.SaveJob(job); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteJob("job-delete"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}

	_, err = store.GetJob("job-delete")
	if err == nil {
		t.Error("expected error after delete")
	}

	pending, _ := store.ListPendingJobs()
	if len(pending) != 0 {
		t.Error("Pending index not cleaned up")
	}
}

func TestSaveFlashRecord(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rec := &FlashRecord{
		ID:           "flash-1",
		DeviceID:     "esp-flash-test",
		DevicePath:   "/dev/ttyUSB0",
		FirmwarePath: "/firmware.bin",
		Status:       FlashStatusCompleted,
		Success:      true,
	}

	if err := store.SaveFlashRecord(rec); err != nil {
		t.Fatalf("SaveFlashRecord: %v", err)
	}

	if rec.StartedAt.IsZero() {
		t.Error("StartedAt not set")
	}
}

func TestGetFlashHistory(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now()

	rec := &FlashRecord{
		ID:           "flash-hist-1",
		DeviceID:     "esp-hist-test",
		DevicePath:   "/dev/ttyUSB0",
		FirmwarePath: "/firmware.bin",
		Status:       FlashStatusCompleted,
		Success:      true,
		StartedAt:    now,
	}

	if err := store.SaveFlashRecord(rec); err != nil {
		t.Fatal(err)
	}

	yearMonth := now.Format("2006-01")
	hist, err := store.GetFlashHistory("esp-hist-test", yearMonth)
	if err != nil {
		t.Fatalf("GetFlashHistory: %v", err)
	}

	if len(hist) != 1 {
		t.Errorf("GetFlashHistory returned %d records, want 1", len(hist))
	}
}

func TestBackup(t *testing.T) {
	tmpdb := tempDB(t)
	store, err := Open(DefaultConfig(tmpdb))
	if err != nil {
		t.Fatal(err)
	}

	dev := &DeviceRecord{
		DeviceID: "esp-backup-test",
		ChipType: "ESP32",
	}

	if err := store.SaveDevice(dev); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(t.TempDir(), "backup.db")
	if err := store.Backup(backupPath); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file not created")
	}

	store.Close()

	restoreStore, err := Open(DefaultConfig(backupPath))
	if err != nil {
		t.Fatalf("Open backup: %v", err)
	}
	defer restoreStore.Close()

	dev, err = restoreStore.GetDevice("esp-backup-test")
	if err != nil {
		t.Errorf("GetDevice from backup: %v", err)
	}
}

func TestConcurrentWrites(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			dev := &DeviceRecord{
				DeviceID:   fmt.Sprintf("esp-concurrent-%d", n),
				MACAddress: fmt.Sprintf("00:00:00:00:00:%02x", n),
				ChipType:   "ESP32",
			}
			if err := store.SaveDevice(dev); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent write error: %v", err)
	}

	list, err := store.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}

	if len(list) != 10 {
		t.Errorf("Expected 10 devices after concurrent writes, got %d", len(list))
	}
}

func TestStats(t *testing.T) {
	store, err := Open(DefaultConfig(tempDB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dev := &DeviceRecord{
		DeviceID: "esp-stats",
		ChipType: "ESP32",
	}

	if err := store.SaveDevice(dev); err != nil {
		t.Fatal(err)
	}

	stats := store.Stats()
	if stats.TxStats.PageCount == 0 {
		t.Error("No stats recorded")
	}
}
