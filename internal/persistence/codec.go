package persistence

import (
	"encoding/json"
	"fmt"
)

type codec struct{}

func (c *codec) Encode(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json encode: %w", err)
	}
	return data, nil
}

func (c *codec) MustEncode(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func (c *codec) Decode(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("json decode: %w", err)
	}
	return nil
}

func (c *codec) DecodeDevice(data []byte) (*DeviceRecord, error) {
	var dev DeviceRecord
	if err := c.Decode(data, &dev); err != nil {
		return nil, err
	}
	return &dev, nil
}

func (c *codec) DecodeJob(data []byte) (*JobRecord, error) {
	var job JobRecord
	if err := c.Decode(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *codec) DecodeFlashRecord(data []byte) (*FlashRecord, error) {
	var rec FlashRecord
	if err := c.Decode(data, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (c *codec) DecodeBoundingBoxMapping(data []byte) (*DeviceBoundingBoxMapping, error) {
	var mapping DeviceBoundingBoxMapping
	if err := c.Decode(data, &mapping); err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (c *codec) DecodeCameraCalibration(data []byte) (*CameraCalibration, error) {
	var calib CameraCalibration
	if err := c.Decode(data, &calib); err != nil {
		return nil, err
	}
	return &calib, nil
}
