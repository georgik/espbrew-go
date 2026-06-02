package protocol

import (
	"encoding/json"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/flashhash"
)

func TestFlashHashQuerySerialization(t *testing.T) {
	tests := []struct {
		name    string
		query   FlashHashQuery
		wantErr bool
	}{
		{
			name: "minimal query",
			query: FlashHashQuery{
				DeviceID:   "esp-aa:bb:cc:dd:ee:ff",
				Regions:    []flashhash.FlashRegionInfo{},
				ClientMode: "fast",
			},
			wantErr: false,
		},
		{
			name: "query with job ID",
			query: FlashHashQuery{
				DeviceID:   "esp-11:22:33:44:55:66",
				JobID:      "job-12345",
				Regions:    []flashhash.FlashRegionInfo{},
				ClientMode: "standard",
			},
			wantErr: false,
		},
		{
			name: "query with regions",
			query: FlashHashQuery{
				DeviceID: "esp-aa:bb:cc:dd:ee:ff",
				JobID:    "job-67890",
				Regions: []flashhash.FlashRegionInfo{
					{Name: flashhash.RegionBootloader, Offset: 0x1000, Size: 0x7000, MD5: "d41d8cd98f00b204e9800998ecf8427e"},
					{Name: flashhash.RegionApplication, Offset: 0x10000, Size: 0x400000, MD5: "5d41402abc4b2a76b9719d911017c592"},
				},
				ClientMode: "accurate",
			},
			wantErr: false,
		},
		{
			name: "query with all ESP32-S3 regions",
			query: FlashHashQuery{
				DeviceID:   "esp-12:34:56:78:9a:bc",
				JobID:      "job-full-s3",
				Regions:    flashhash.StandardESP32S3Layout4MB(),
				ClientMode: "fast",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test serialization
			data, err := json.Marshal(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Test deserialization
				var decoded FlashHashQuery
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Errorf("Unmarshal() error = %v", err)
					return
				}

				// Verify critical fields
				if decoded.DeviceID != tt.query.DeviceID {
					t.Errorf("DeviceID = %v, want %v", decoded.DeviceID, tt.query.DeviceID)
				}
				if decoded.JobID != tt.query.JobID {
					t.Errorf("JobID = %v, want %v", decoded.JobID, tt.query.JobID)
				}
				if decoded.ClientMode != tt.query.ClientMode {
					t.Errorf("ClientMode = %v, want %v", decoded.ClientMode, tt.query.ClientMode)
				}
				if len(decoded.Regions) != len(tt.query.Regions) {
					t.Errorf("Regions length = %v, want %v", len(decoded.Regions), len(tt.query.Regions))
				}
			}
		})
	}
}

func TestFlashHashResponseSerialization(t *testing.T) {
	tests := []struct {
		name     string
		response FlashHashResponse
		wantErr  bool
	}{
		{
			name: "full flash required",
			response: FlashHashResponse{
				Status:            "full_flash",
				RegionsNeeded:     []flashhash.FlashRegionInfo{},
				RegionsCached:     []flashhash.CachedRegionInfo{},
				RecommendedAction: "flash_all",
			},
			wantErr: false,
		},
		{
			name: "partial update with regions needed",
			response: FlashHashResponse{
				Status: "partial_update",
				RegionsNeeded: []flashhash.FlashRegionInfo{
					{Name: flashhash.RegionApplication, Offset: 0x10000, Size: 0x400000, MD5: "5d41402abc4b2a76b9719d911017c592"},
				},
				RegionsCached: []flashhash.CachedRegionInfo{
					{Name: flashhash.RegionBootloader, Reason: "hash_match"},
					{Name: flashhash.RegionPartitionTable, Reason: "hash_match"},
				},
				JobID:             "job-12345",
				RecommendedAction: "flash_application",
			},
			wantErr: false,
		},
		{
			name: "skip all regions",
			response: FlashHashResponse{
				Status:        "skip_all",
				RegionsNeeded: []flashhash.FlashRegionInfo{},
				RegionsCached: []flashhash.CachedRegionInfo{
					{Name: flashhash.RegionBootloader, Reason: "hash_match"},
					{Name: flashhash.RegionPartitionTable, Reason: "hash_match"},
					{Name: flashhash.RegionOTASelect, Reason: "hash_match"},
					{Name: flashhash.RegionApplication, Reason: "hash_match"},
				},
				Message:           "All regions match, no flashing required",
				RecommendedAction: "skip",
			},
			wantErr: false,
		},
		{
			name: "response with error message",
			response: FlashHashResponse{
				Status:            "error",
				Message:           "Failed to read flash: device not found",
				RecommendedAction: "retry",
			},
			wantErr: false,
		},
		{
			name: "complete response with all fields",
			response: FlashHashResponse{
				Status: "partial_update",
				RegionsNeeded: []flashhash.FlashRegionInfo{
					{Name: flashhash.RegionApplication, Offset: 0x10000, Size: 0x400000, MD5: "5d41402abc4b2a76b9719d911017c592"},
					{Name: flashhash.RegionNVS, Offset: 0x9000, Size: 0x6000, MD5: "098f6bcd4621d373cade4e832627b4f6"},
				},
				RegionsCached: []flashhash.CachedRegionInfo{
					{Name: flashhash.RegionBootloader, Reason: "hash_match"},
				},
				JobID:             "job-complex-123",
				Message:           "2 regions need updating, 1 region cached",
				RecommendedAction: "flash_selected",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test serialization
			data, err := json.Marshal(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Test deserialization
				var decoded FlashHashResponse
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Errorf("Unmarshal() error = %v", err)
					return
				}

				// Verify critical fields
				if decoded.Status != tt.response.Status {
					t.Errorf("Status = %v, want %v", decoded.Status, tt.response.Status)
				}
				if decoded.JobID != tt.response.JobID {
					t.Errorf("JobID = %v, want %v", decoded.JobID, tt.response.JobID)
				}
				if decoded.Message != tt.response.Message {
					t.Errorf("Message = %v, want %v", decoded.Message, tt.response.Message)
				}
				if decoded.RecommendedAction != tt.response.RecommendedAction {
					t.Errorf("RecommendedAction = %v, want %v", decoded.RecommendedAction, tt.response.RecommendedAction)
				}
				if len(decoded.RegionsNeeded) != len(tt.response.RegionsNeeded) {
					t.Errorf("RegionsNeeded length = %v, want %v", len(decoded.RegionsNeeded), len(tt.response.RegionsNeeded))
				}
				if len(decoded.RegionsCached) != len(tt.response.RegionsCached) {
					t.Errorf("RegionsCached length = %v, want %v", len(decoded.RegionsCached), len(tt.response.RegionsCached))
				}
			}
		})
	}
}

func TestFlashHashQueryRoundTrip(t *testing.T) {
	original := FlashHashQuery{
		DeviceID: "esp-test-round-trip",
		JobID:    "job-round-trip-123",
		Regions: []flashhash.FlashRegionInfo{
			{Name: flashhash.RegionBootloader, Offset: 0x1000, Size: 0x7000, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{Name: flashhash.RegionPartitionTable, Offset: 0x8000, Size: 0x1000, MD5: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
			{Name: flashhash.RegionOTASelect, Offset: 0x9000, Size: 0x2000, MD5: "cccccccccccccccccccccccccccccccc"},
			{Name: flashhash.RegionApplication, Offset: 0x10000, Size: 0x400000, MD5: "dddddddddddddddddddddddddddddddd"},
		},
		ClientMode: "accurate",
	}

	// Serialize
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Deserialize
	var decoded FlashHashQuery
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify all fields match
	if decoded.DeviceID != original.DeviceID {
		t.Errorf("DeviceID mismatch: got %v, want %v", decoded.DeviceID, original.DeviceID)
	}
	if decoded.JobID != original.JobID {
		t.Errorf("JobID mismatch: got %v, want %v", decoded.JobID, original.JobID)
	}
	if decoded.ClientMode != original.ClientMode {
		t.Errorf("ClientMode mismatch: got %v, want %v", decoded.ClientMode, original.ClientMode)
	}

	if len(decoded.Regions) != len(original.Regions) {
		t.Fatalf("Regions length mismatch: got %d, want %d", len(decoded.Regions), len(original.Regions))
	}

	for i := range original.Regions {
		if decoded.Regions[i].Name != original.Regions[i].Name {
			t.Errorf("Region[%d].Name mismatch: got %v, want %v", i, decoded.Regions[i].Name, original.Regions[i].Name)
		}
		if decoded.Regions[i].Offset != original.Regions[i].Offset {
			t.Errorf("Region[%d].Offset mismatch: got %v, want %v", i, decoded.Regions[i].Offset, original.Regions[i].Offset)
		}
		if decoded.Regions[i].Size != original.Regions[i].Size {
			t.Errorf("Region[%d].Size mismatch: got %v, want %v", i, decoded.Regions[i].Size, original.Regions[i].Size)
		}
		if decoded.Regions[i].MD5 != original.Regions[i].MD5 {
			t.Errorf("Region[%d].MD5 mismatch: got %v, want %v", i, decoded.Regions[i].MD5, original.Regions[i].MD5)
		}
	}
}

func TestFlashHashResponseRoundTrip(t *testing.T) {
	original := FlashHashResponse{
		Status: "partial_update",
		RegionsNeeded: []flashhash.FlashRegionInfo{
			{Name: flashhash.RegionApplication, Offset: 0x10000, Size: 0x400000, MD5: "11111111111111111111111111111111"},
		},
		RegionsCached: []flashhash.CachedRegionInfo{
			{Name: flashhash.RegionBootloader, Reason: "hash_match"},
			{Name: flashhash.RegionPartitionTable, Reason: "hash_match"},
			{Name: flashhash.RegionOTASelect, Reason: "not_in_job"},
		},
		JobID:             "job-response-456",
		Message:           "Optimized: 1 region to flash, 2 cached",
		RecommendedAction: "flash_selected",
	}

	// Serialize
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Deserialize
	var decoded FlashHashResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify all fields match
	if decoded.Status != original.Status {
		t.Errorf("Status mismatch: got %v, want %v", decoded.Status, original.Status)
	}
	if decoded.JobID != original.JobID {
		t.Errorf("JobID mismatch: got %v, want %v", decoded.JobID, original.JobID)
	}
	if decoded.Message != original.Message {
		t.Errorf("Message mismatch: got %v, want %v", decoded.Message, original.Message)
	}
	if decoded.RecommendedAction != original.RecommendedAction {
		t.Errorf("RecommendedAction mismatch: got %v, want %v", decoded.RecommendedAction, original.RecommendedAction)
	}

	if len(decoded.RegionsNeeded) != len(original.RegionsNeeded) {
		t.Fatalf("RegionsNeeded length mismatch: got %d, want %d", len(decoded.RegionsNeeded), len(original.RegionsNeeded))
	}

	for i := range original.RegionsNeeded {
		if decoded.RegionsNeeded[i].Name != original.RegionsNeeded[i].Name {
			t.Errorf("RegionsNeeded[%d].Name mismatch: got %v, want %v", i, decoded.RegionsNeeded[i].Name, original.RegionsNeeded[i].Name)
		}
		if decoded.RegionsNeeded[i].Offset != original.RegionsNeeded[i].Offset {
			t.Errorf("RegionsNeeded[%d].Offset mismatch: got %v, want %v", i, decoded.RegionsNeeded[i].Offset, original.RegionsNeeded[i].Offset)
		}
		if decoded.RegionsNeeded[i].Size != original.RegionsNeeded[i].Size {
			t.Errorf("RegionsNeeded[%d].Size mismatch: got %v, want %v", i, decoded.RegionsNeeded[i].Size, original.RegionsNeeded[i].Size)
		}
		if decoded.RegionsNeeded[i].MD5 != original.RegionsNeeded[i].MD5 {
			t.Errorf("RegionsNeeded[%d].MD5 mismatch: got %v, want %v", i, decoded.RegionsNeeded[i].MD5, original.RegionsNeeded[i].MD5)
		}
	}

	if len(decoded.RegionsCached) != len(original.RegionsCached) {
		t.Fatalf("RegionsCached length mismatch: got %d, want %d", len(decoded.RegionsCached), len(original.RegionsCached))
	}

	for i := range original.RegionsCached {
		if decoded.RegionsCached[i].Name != original.RegionsCached[i].Name {
			t.Errorf("RegionsCached[%d].Name mismatch: got %v, want %v", i, decoded.RegionsCached[i].Name, original.RegionsCached[i].Name)
		}
		if decoded.RegionsCached[i].Reason != original.RegionsCached[i].Reason {
			t.Errorf("RegionsCached[%d].Reason mismatch: got %v, want %v", i, decoded.RegionsCached[i].Reason, original.RegionsCached[i].Reason)
		}
	}
}

func TestFlashHashQueryWithMessageWrapper(t *testing.T) {
	query := FlashHashQuery{
		DeviceID: "esp-message-test",
		JobID:    "job-msg-789",
		Regions: []flashhash.FlashRegionInfo{
			{Name: flashhash.RegionBootloader, Offset: 0x1000, Size: 0x7000, MD5: "testbootloaderhash1234567890abcdef"},
		},
		ClientMode: "fast",
	}

	msg := Message{
		Type:    MsgFlashHashQuery,
		Payload: query,
	}

	// Serialize the wrapped message
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal wrapped message failed: %v", err)
	}

	// Deserialize the wrapped message
	var decodedMsg Message
	if err := json.Unmarshal(data, &decodedMsg); err != nil {
		t.Fatalf("Unmarshal wrapped message failed: %v", err)
	}

	// Verify message type
	if decodedMsg.Type != MsgFlashHashQuery {
		t.Errorf("Message type = %v, want %v", decodedMsg.Type, MsgFlashHashQuery)
	}

	// Extract payload
	payloadBytes, err := json.Marshal(decodedMsg.Payload)
	if err != nil {
		t.Fatalf("Marshal payload failed: %v", err)
	}

	var decodedQuery FlashHashQuery
	if err := json.Unmarshal(payloadBytes, &decodedQuery); err != nil {
		t.Fatalf("Unmarshal payload failed: %v", err)
	}

	// Verify query fields
	if decodedQuery.DeviceID != query.DeviceID {
		t.Errorf("DeviceID = %v, want %v", decodedQuery.DeviceID, query.DeviceID)
	}
	if decodedQuery.ClientMode != query.ClientMode {
		t.Errorf("ClientMode = %v, want %v", decodedQuery.ClientMode, query.ClientMode)
	}
}

func TestFlashHashResponseWithMessageWrapper(t *testing.T) {
	response := FlashHashResponse{
		Status:        "partial_update",
		RegionsNeeded: []flashhash.FlashRegionInfo{},
		RegionsCached: []flashhash.CachedRegionInfo{
			{Name: flashhash.RegionBootloader, Reason: "hash_match"},
		},
		JobID:             "job-wrap-test",
		Message:           "Test message wrapper",
		RecommendedAction: "skip",
	}

	msg := Message{
		Type:    MsgFlashHashResponse,
		Payload: response,
	}

	// Serialize the wrapped message
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal wrapped message failed: %v", err)
	}

	// Deserialize the wrapped message
	var decodedMsg Message
	if err := json.Unmarshal(data, &decodedMsg); err != nil {
		t.Fatalf("Unmarshal wrapped message failed: %v", err)
	}

	// Verify message type
	if decodedMsg.Type != MsgFlashHashResponse {
		t.Errorf("Message type = %v, want %v", decodedMsg.Type, MsgFlashHashResponse)
	}

	// Extract payload
	payloadBytes, err := json.Marshal(decodedMsg.Payload)
	if err != nil {
		t.Fatalf("Marshal payload failed: %v", err)
	}

	var decodedResponse FlashHashResponse
	if err := json.Unmarshal(payloadBytes, &decodedResponse); err != nil {
		t.Fatalf("Unmarshal payload failed: %v", err)
	}

	// Verify response fields
	if decodedResponse.Status != response.Status {
		t.Errorf("Status = %v, want %v", decodedResponse.Status, response.Status)
	}
	if decodedResponse.RecommendedAction != response.RecommendedAction {
		t.Errorf("RecommendedAction = %v, want %v", decodedResponse.RecommendedAction, response.RecommendedAction)
	}
}
