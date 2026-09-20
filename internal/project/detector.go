package project

// ProjectType identifies the type of ESP project
type ProjectType string

const (
	ProjectTypeNone       ProjectType = ""
	ProjectTypeESPIDF     ProjectType = "esp-idf"
	ProjectTypeRustESP    ProjectType = "rust-esp"
	ProjectTypeTinyGo     ProjectType = "tinygo"
	ProjectTypePlatformIO ProjectType = "platformio" // Future
	ProjectTypeArduino    ProjectType = "arduino"    // Future
)

// FlashFile is a single image to flash together with the exact offset the build
// assigned to it in flash_args. The ordered slice BuildArtifacts.FlashFiles is the
// authoritative flash plan for an ESP-IDF project.
type FlashFile struct {
	// Path is the absolute path to the image on disk.
	Path string
	// Offset is the flash offset taken from flash_args.
	Offset uint32
	// Name is a human-friendly label (usually the file base name) used for logging.
	Name string
}

// ExtraFile is a partition image discovered in flash_args that is NOT one of the
// standard bootloader/partitions/app slots. These are extra data partitions that
// ESP-IDF can emit alongside the app, e.g. a pre-populated FAT image produced by
// fatfs_create_spiflash_image (storage.bin) or a separate NVS/OTA table. The
// standard three-slot detection would silently skip these, so they must be
// tracked separately and flashed explicitly.
type ExtraFile struct {
	// Path is the absolute path to the partition image on disk.
	Path string
	// Offset is the flash offset taken from flash_args.
	Offset uint32
	// Name is a human-friendly label (usually the file base name) used for logging.
	Name string
}

// BuildArtifacts contains paths to build outputs
type BuildArtifacts struct {
	BuildDir   string
	Bootloader string
	Partitions string
	App        string
	FlashArgs  string
	// ExtraFiles holds additional partition images (beyond bootloader/partitions/app)
	// discovered by honoring the build's flash_args / partition table. When empty the
	// project only needs the standard three images.
	ExtraFiles []ExtraFile
	// FlashFiles is the authoritative, ordered list of every image the build wants
	// flashed, together with the exact flash offset from flash_args. This is the
	// source of truth for flashing (equivalent to what `idf.py flash` does) and must
	// drive the flasher so that each image lands at its real partition offset rather
	// than a preset one. When empty (e.g. explicit --bootloader/--partitions/--app
	// without a discoverable flash_args), the caller falls back to preset offsets.
	FlashFiles []FlashFile
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
