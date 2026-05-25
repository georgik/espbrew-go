package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockDetector for testing
type MockDetector struct {
	NameValue       string
	DetectValue     bool
	BuildDirValue   string
	ArtifactsValue  *BuildArtifacts
	FindBuildDirErr error
	GetArtifactsErr error
}

func (m *MockDetector) Name() string {
	if m.NameValue != "" {
		return m.NameValue
	}
	return "mock"
}

func (m *MockDetector) Detect(dir string) bool {
	return m.DetectValue
}

func (m *MockDetector) FindBuildDir(dir string) (string, error) {
	if m.FindBuildDirErr != nil {
		return "", m.FindBuildDirErr
	}
	return m.BuildDirValue, nil
}

func (m *MockDetector) GetArtifacts(buildDir string) (*BuildArtifacts, error) {
	if m.GetArtifactsErr != nil {
		return nil, m.GetArtifactsErr
	}
	return m.ArtifactsValue, nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	assert.NotNil(t, registry)
	assert.Empty(t, registry.detectors)
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	mock := &MockDetector{NameValue: "test"}

	registry.Register(mock)

	assert.Len(t, registry.detectors, 1)
	assert.Same(t, mock, registry.detectors[0])
}

func TestRegistry_Detect(t *testing.T) {
	tests := []struct {
		name       string
		detectors  []Detector
		wantType   ProjectType
		wantDetect bool
	}{
		{
			name: "detector matches",
			detectors: []Detector{
				&MockDetector{NameValue: "esp-idf", DetectValue: true},
			},
			wantType:   ProjectTypeESPIDF,
			wantDetect: true,
		},
		{
			name: "no detector matches",
			detectors: []Detector{
				&MockDetector{NameValue: "esp-idf", DetectValue: false},
			},
			wantType:   ProjectTypeNone,
			wantDetect: false,
		},
		{
			name: "first matching detector wins",
			detectors: []Detector{
				&MockDetector{NameValue: "first", DetectValue: true},
				&MockDetector{NameValue: "second", DetectValue: true},
			},
			wantType:   "first",
			wantDetect: true,
		},
		{
			name:       "no detectors registered",
			detectors:  []Detector{},
			wantType:   ProjectTypeNone,
			wantDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			for _, d := range tt.detectors {
				registry.Register(d)
			}

			projType, detector := registry.Detect("any-dir")

			if tt.wantDetect {
				assert.Equal(t, tt.wantType, projType)
				assert.NotNil(t, detector)
				assert.Equal(t, tt.wantType, ProjectType(detector.Name()))
			} else {
				assert.Equal(t, ProjectTypeNone, projType)
				assert.Nil(t, detector)
			}
		})
	}
}
