package project

// ProjectType identifies the type of ESP project
type ProjectType string

const (
	ProjectTypeNone    ProjectType = ""
	ProjectTypeESPIDF  ProjectType = "esp-idf"
	ProjectTypePlatformIO ProjectType = "platformio" // Future
	ProjectTypeArduino ProjectType = "arduino"       // Future
)

// BuildArtifacts contains paths to build outputs
type BuildArtifacts struct {
	BuildDir   string
	Bootloader string
	Partitions string
	App        string
	FlashArgs  string
}

// Detector can identify and extract build artifacts from a project
type Detector interface {
	// Name returns the detector name
	Name() string

	// Detect checks if the current directory matches this project type
	Detect(dir string) bool

	// FindBuildDir locates the build directory
	FindBuildDir(dir string) (string, error)

	// GetArtifacts returns paths to bootloader, partitions, app binaries
	GetArtifacts(buildDir string) (*BuildArtifacts, error)
}

// Registry holds all registered detectors
type Registry struct {
	detectors []Detector
}

// NewRegistry creates a new detector registry
func NewRegistry() *Registry {
	return &Registry{
		detectors: []Detector{},
	}
}

// Register adds a detector to the registry
func (r *Registry) Register(d Detector) {
	r.detectors = append(r.detectors, d)
}

// Detect finds the project type and returns the appropriate detector
func (r *Registry) Detect(dir string) (ProjectType, Detector) {
	for _, d := range r.detectors {
		if d.Detect(dir) {
			return ProjectType(d.Name()), d
		}
	}
	return ProjectTypeNone, nil
}
