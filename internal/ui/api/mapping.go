//go:build js
// +build js

package api

import (
	"syscall/js"
)

// GetCameraMappings retrieves mappings for a camera
func GetCameraMappings(cameraID string, callback func(*CameraMappingsResponse, error)) {
	DefaultAsyncClient.Get("/cameras/"+cameraID+"/boxes", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		response := &CameraMappingsResponse{}
		if err := ParseJSONValue(result, response); err != nil {
			callback(nil, err)
			return
		}

		callback(response, nil)
	})
}

// CreateBoundingBox creates a new bounding box mapping
func CreateBoundingBox(mapping DeviceBoundingBoxMapping, callback func(*DeviceBoundingBoxMapping, error)) {
	DefaultAsyncClient.Post("/bounding_boxes", mapping, func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		created := &DeviceBoundingBoxMapping{}
		if err := ParseJSONValue(result, created); err != nil {
			callback(nil, err)
			return
		}

		callback(created, nil)
	})
}

// UpdateBoundingBox updates an existing bounding box mapping
func UpdateBoundingBox(mappingID string, update map[string]interface{}, callback func(*DeviceBoundingBoxMapping, error)) {
	DefaultAsyncClient.Put("/bounding_boxes/"+mappingID, update, func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		updated := &DeviceBoundingBoxMapping{}
		if err := ParseJSONValue(result, updated); err != nil {
			callback(nil, err)
			return
		}

		callback(updated, nil)
	})
}

// DeleteBoundingBox deletes a bounding box mapping
func DeleteBoundingBox(mappingID string, callback func(error)) {
	DefaultAsyncClient.Delete("/bounding_boxes/"+mappingID, func(result js.Value, err error) {
		callback(err)
	})
}

// GetCalibration retrieves calibration for a camera
func GetCalibration(cameraID string, callback func(*CalibrationInfo, error)) {
	DefaultAsyncClient.Get("/cameras/"+cameraID+"/calibration", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		calib := &CalibrationInfo{}
		if err := ParseJSONValue(result, calib); err != nil {
			callback(nil, err)
			return
		}

		callback(calib, nil)
	})
}

// CreateCalibration creates a new calibration version
func CreateCalibration(cameraID, description string, callback func(*CalibrationInfo, error)) {
	req := map[string]interface{}{
		"description": description,
	}

	DefaultAsyncClient.Post("/cameras/"+cameraID+"/calibration", req, func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		calib := &CalibrationInfo{}
		if err := ParseJSONValue(result, calib); err != nil {
			callback(nil, err)
			return
		}

		callback(calib, nil)
	})
}

// CreateMapping creates a new device mapping (simplified version)
func CreateMapping(req CreateMappingRequest, callback func(*CreateMappingResponse, error)) {
	mapping := DeviceBoundingBoxMapping{
		DeviceID:   req.DeviceID,
		CameraID:   req.CameraID,
		CameraName: req.CameraName, // Pass through stable camera identifier
		Bounds:     req.Bounds,
	}

	DefaultAsyncClient.Post("/bounding_boxes", mapping, func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		resp := &CreateMappingResponse{}
		if err := ParseJSONValue(result, resp); err != nil {
			callback(nil, err)
		}

		callback(resp, nil)
	})
}
