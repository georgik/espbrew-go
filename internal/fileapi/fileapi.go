//go:build js
// +build js

// Package fileapi provides file system access for WASM using browser APIs
package fileapi

import (
	"strings"
	"syscall/js"
)

// BrowserSupport indicates which file APIs are available
type BrowserSupport struct {
	FileSystemAccessAPI bool
	WebkitDirectory     bool
	InputFiles          bool
}

// FolderFiles represents files selected from a folder
type FolderFiles struct {
	Name      string              // Folder name
	Files     map[string][]byte   // filename -> content (relative paths)
	Handles   map[string]js.Value // For re-access (File System Access API)
	DirHandle js.Value            // Directory handle for selective reading
}

// FolderPicker handles folder selection in browser
type FolderPicker struct {
	supported BrowserSupport
}

// NewFolderPicker creates a new folder picker
func NewFolderPicker() *FolderPicker {
	return &FolderPicker{
		supported: checkBrowserSupport(),
	}
}

// checkBrowserSupport determines available file APIs
func checkBrowserSupport() BrowserSupport {
	window := js.Global().Get("window")
	document := js.Global().Get("document")

	// File System Access API requires both the API to exist AND secure context
	hasAPI := window.Get("showDirectoryPicker").Truthy()
	isSecureContext := window.Get("isSecureContext").Truthy()

	return BrowserSupport{
		FileSystemAccessAPI: hasAPI && isSecureContext,
		WebkitDirectory:     true,              // Attribute works in most browsers
		InputFiles:          document.Truthy(), // Always available
	}
}

// SelectFolder prompts user to select a folder and returns its files via callback
func (fp *FolderPicker) SelectFolder(callback func(*FolderFiles, error)) {
	// Try File System Access API first (best UX)
	if fp.supported.FileSystemAccessAPI {
		fp.selectWithAPI(func(files *FolderFiles, err error) {
			if err == nil && len(files.Files) > 0 {
				callback(files, nil)
			} else {
				// Fall back to webkitdirectory on error
				fp.selectWithWebkitDirectory(callback)
			}
		})
		return
	}

	// Fallback to webkitdirectory
	if fp.supported.WebkitDirectory {
		fp.selectWithWebkitDirectory(callback)
		return
	}

	// Last resort: multiple file input (user selects all files manually)
	fp.selectWithMultipleInput(callback)
}

// selectWithAPI uses File System Access API with callback
func (fp *FolderPicker) selectWithAPI(callback func(*FolderFiles, error)) {
	window := js.Global().Get("window")

	// Call showDirectoryPicker()
	promise := window.Call("showDirectoryPicker")

	// Handle promise with proper async callbacks
	thenFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
			callback(nil, ErrInvalidPromise)
			return nil
		}

		dirHandle := args[0]

		// Run async operations in goroutine to avoid blocking UI thread
		go func() {
			files, err := readDirectory(dirHandle)
			callback(files, err)
		}()

		return nil
	})

	catchFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			callback(nil, &JSError{Value: args[0]})
		} else {
			callback(nil, ErrInvalidPromise)
		}
		return nil
	})

	promise.Call("then", thenFunc).Call("catch", catchFunc)

	// Note: Don't release thenFunc/catchFunc - they're needed for the promise lifecycle
	// The Go GC will eventually collect them
}

// selectWithWebkitDirectory uses input with webkitdirectory attribute
func (fp *FolderPicker) selectWithWebkitDirectory(callback func(*FolderFiles, error)) {
	doc := js.Global().Get("document")
	body := doc.Get("body")

	// Create hidden input element
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("webkitdirectory", "true")
	input.Set("style", "display:none")
	input.Set("id", "folder-picker-input")

	// Add to DOM
	body.Call("appendChild", input)

	var listener js.Func

	// Set up event listener
	listener = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		files := input.Get("files")

		// Clean up DOM
		body.Call("removeChild", input)

		// Release this listener
		listener.Release()

		if files.IsUndefined() || files.IsNull() || files.Get("length").Int() == 0 {
			callback(nil, ErrNoFilesSelected)
		} else {
			// Process files asynchronously
			go func() {
				result, err := readInputFiles(files)
				callback(result, err)
			}()
		}

		return nil
	})

	input.Call("addEventListener", "change", listener)

	// Trigger click
	input.Call("click")
}

// selectWithMultipleInput uses regular multiple file input as last resort
func (fp *FolderPicker) selectWithMultipleInput(callback func(*FolderFiles, error)) {
	doc := js.Global().Get("document")
	body := doc.Get("body")

	// Create input element
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("multiple", "true")
	input.Set("style", "display:none")

	// Add to DOM
	body.Call("appendChild", input)

	var listener js.Func

	// Set up event listener
	listener = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		files := input.Get("files")

		// Clean up DOM
		body.Call("removeChild", input)

		// Release this listener
		listener.Release()

		if files.IsUndefined() || files.IsNull() || files.Get("length").Int() == 0 {
			callback(nil, ErrNoFilesSelected)
		} else {
			// Process files asynchronously
			go func() {
				result, err := readInputFiles(files)
				callback(result, err)
			}()
		}

		return nil
	})

	input.Call("addEventListener", "change", listener)

	// Trigger click
	input.Call("click")
}

// readDirectory reads only marker files from a directory handle (callback version)
func readDirectory(dirHandle js.Value) (*FolderFiles, error) {
	files := &FolderFiles{
		Files:     make(map[string][]byte),
		Handles:   make(map[string]js.Value),
		DirHandle: dirHandle,
	}

	// Validate dirHandle
	if dirHandle.IsUndefined() || dirHandle.IsNull() {
		return nil, ErrInvalidPromise
	}

	// Try to get directory name
	if name := dirHandle.Get("name"); name.Truthy() {
		files.Name = name.String()
	}

	// Marker files for project detection (root level only)
	markerFiles := map[string]bool{
		"CMakeLists.txt":     true,
		"sdkconfig":          true,
		"sdkconfig.defaults": true,
		"Cargo.toml":         true,
		"go.mod":             true,
		"main.go":            true,
	}

	// Marker files in subdirectories (prefix -> set of basenames)
	subdirMarkers := map[string]map[string]bool{
		".cargo": {
			"config.toml": true,
			"config":      true,
		},
		"build": {
			"build.ninja": true,
		},
	}

	// Iterate directory entries synchronously using helper
	entries := getDirectoryEntries(dirHandle)

	for _, entry := range entries {
		if entry.IsUndefined() || entry.IsNull() {
			continue
		}

		kind := entry.Get("kind").String()
		name := entry.Get("name").String()

		if kind == "file" {
			// Check if it's a root-level marker file
			if markerFiles[name] {
				data, err := readFileFromEntry(entry)
				if err == nil && data != nil {
					files.Files[name] = data
					files.Handles[name] = entry
				}
			}
		} else if kind == "directory" {
			// Check if this is a directory with marker files
			if markers, hasMarkers := subdirMarkers[name]; hasMarkers {
				// Recursively read subdirectory and look for marker files
				subdirPromise := dirHandle.Call("getDirectoryHandle", name)
				subdir, err := awaitPromiseSync(subdirPromise)
				if err == nil && !subdir.IsUndefined() && !subdir.IsNull() {
					subFiles, _ := readDirectoryWithMarkers(subdir, markers)
					// Merge with prefix
					for n, data := range subFiles.Files {
						files.Files[name+"/"+n] = data
					}
					for n, handle := range subFiles.Handles {
						files.Handles[name+"/"+n] = handle
					}
				}
			}
		}
	}

	return files, nil
}

// readDirectoryWithMarkers reads only specified marker files from a directory
func readDirectoryWithMarkers(dirHandle js.Value, markers map[string]bool) (*FolderFiles, error) {
	files := &FolderFiles{
		Files:     make(map[string][]byte),
		Handles:   make(map[string]js.Value),
		DirHandle: dirHandle,
	}

	// Validate dirHandle
	if dirHandle.IsUndefined() || dirHandle.IsNull() {
		return nil, ErrInvalidPromise
	}

	// Iterate directory entries
	entries := getDirectoryEntries(dirHandle)

	for _, entry := range entries {
		if entry.IsUndefined() || entry.IsNull() {
			continue
		}

		kind := entry.Get("kind").String()
		name := entry.Get("name").String()

		if kind == "file" && markers[name] {
			// Read file content
			data, err := readFileFromEntry(entry)
			if err == nil && data != nil {
				files.Files[name] = data
				files.Handles[name] = entry
			}
		}
	}

	return files, nil
}

// FindELFFilesInTarget checks known artifact paths for ELF files
// Returns list of paths (relative to dirHandle) for found ELF files
func (ff *FolderFiles) FindELFFilesInTarget(targetTriple string) []string {
	if ff.DirHandle.IsUndefined() || ff.DirHandle.IsNull() {
		return nil
	}

	var elfPaths []string

	// Known Rust artifact paths to check (in order of preference)
	knownPaths := []string{
		"target/" + targetTriple + "/release/",
		"target/release/",
		"build/",
	}

	for _, basePath := range knownPaths {
		// Try to get the directory
		dirPromise := ff.DirHandle.Call("getDirectoryHandle", strings.Split(basePath, "/")[0])
		dirHandle, err := awaitPromiseSync(dirPromise)
		if err != nil {
			continue
		}

		// For deeper paths, navigate step by step
		parts := strings.Split(basePath, "/")
		currentHandle := dirHandle

		for i := 1; i < len(parts)-1; i++ {
			subPromise := currentHandle.Call("getDirectoryHandle", parts[i])
			currentHandle, err = awaitPromiseSync(subPromise)
			if err != nil {
				currentHandle = js.Undefined()
				break
			}
		}

		if currentHandle.IsUndefined() || currentHandle.IsNull() {
			continue
		}

		// List files in the target directory
		entries := getDirectoryEntries(currentHandle)

		for _, entry := range entries {
			if entry.IsUndefined() || entry.IsNull() {
				continue
			}

			if entry.Get("kind").String() != "file" {
				continue
			}

			name := entry.Get("name").String()

			// Check if it's an ELF file
			data, err := readFileFromEntry(entry)
			if err == nil && isELFFile(data) {
				// Skip build artifacts (.d, .o, .a files)
				if !strings.HasSuffix(name, ".d") &&
					!strings.HasSuffix(name, ".o") &&
					!strings.HasSuffix(name, ".a") &&
					!strings.HasSuffix(name, ".rmeta") &&
					!strings.HasSuffix(name, ".rlib") {
					fullPath := basePath + name
					elfPaths = append(elfPaths, fullPath)
					// Cache the data we just read
					ff.Files[fullPath] = data
				}
			}
		}

		// Found something in this path, stop checking
		if len(elfPaths) > 0 {
			break
		}
	}

	return elfPaths
}

// FindBinFilesInBuild checks build/ directory for .bin files
// Returns list of paths (relative to dirHandle) for found .bin files
func (ff *FolderFiles) FindBinFilesInBuild() []string {
	if ff.DirHandle.IsUndefined() || ff.DirHandle.IsNull() {
		return nil
	}

	var binPaths []string

	// Try to get build directory
	buildPromise := ff.DirHandle.Call("getDirectoryHandle", "build")
	buildHandle, err := awaitPromiseSync(buildPromise)
	if err != nil {
		return nil
	}

	// List files in build directory
	entries := getDirectoryEntries(buildHandle)

	for _, entry := range entries {
		if entry.IsUndefined() || entry.IsNull() {
			continue
		}

		kind := entry.Get("kind").String()
		name := entry.Get("name").String()

		if kind == "file" && strings.HasSuffix(name, ".bin") {
			// Found a .bin file at root level
			fullPath := "build/" + name
			data, err := readFileFromEntry(entry)
			if err == nil && len(data) > 0 {
				binPaths = append(binPaths, fullPath)
				ff.Files[fullPath] = data
			}
		} else if kind == "directory" {
			// Check subdirectories like bootloader, partition_table
			subdirPromise := buildHandle.Call("getDirectoryHandle", name)
			subHandle, err := awaitPromiseSync(subdirPromise)
			if err != nil {
				continue
			}

			subEntries := getDirectoryEntries(subHandle)
			for _, subEntry := range subEntries {
				if subEntry.IsUndefined() || subEntry.IsNull() {
					continue
				}

				if subEntry.Get("kind").String() != "file" {
					continue
				}

				subName := subEntry.Get("name").String()
				if strings.HasSuffix(subName, ".bin") {
					fullPath := "build/" + name + "/" + subName
					data, err := readFileFromEntry(subEntry)
					if err == nil && len(data) > 0 {
						binPaths = append(binPaths, fullPath)
						ff.Files[fullPath] = data
					}
				}
			}
		}
	}

	return binPaths
}

// isELFFile checks if data is an ELF binary
func isELFFile(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
}

// awaitPromiseSync waits for a JS Promise to resolve using channels (yields to JS event loop)
func awaitPromiseSync(promise js.Value) (js.Value, error) {
	if promise.IsUndefined() || promise.IsNull() {
		return js.Undefined(), ErrInvalidPromise
	}

	resCh := make(chan js.Value, 1)
	errCh := make(chan error, 1)

	// Success callback
	resolve := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var res js.Value
		if len(args) > 0 {
			res = args[0]
		}
		resCh <- res
		return nil
	})
	defer resolve.Release()

	// Failure callback
	reject := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			errCh <- &JSError{Value: args[0]}
		} else {
			errCh <- ErrInvalidPromise
		}
		return nil
	})
	defer reject.Release()

	promise.Call("then", resolve, reject)

	// Block on channel - this yields to JS event loop, allowing promise to resolve
	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return js.Undefined(), err
	}
}

// getDirectoryEntries synchronously gets all entries from a directory (max 1000)
func getDirectoryEntries(dirHandle js.Value) []js.Value {
	var entries []js.Value
	const maxEntries = 1000

	if dirHandle.IsUndefined() || dirHandle.IsNull() {
		return entries
	}

	iterator := dirHandle.Call("values")
	if iterator.IsUndefined() || iterator.IsNull() {
		return entries
	}

	for i := 0; i < maxEntries; i++ {
		nextPromise := iterator.Call("next")
		result, err := awaitPromiseSync(nextPromise)
		if err != nil {
			break
		}

		if result.IsUndefined() || result.IsNull() {
			break
		}

		done := result.Get("done")
		if done.Bool() {
			break
		}

		value := result.Get("value")
		if !value.IsUndefined() && !value.IsNull() {
			entries = append(entries, value)
		}
	}

	return entries
}

// readFileFromEntry synchronously reads a file from a directory entry
func readFileFromEntry(entry js.Value) ([]byte, error) {
	if entry.IsUndefined() || entry.IsNull() {
		return nil, ErrInvalidPromise
	}

	filePromise := entry.Call("getFile")
	fileObj, err := awaitPromiseSync(filePromise)
	if err != nil {
		return nil, err
	}

	if fileObj.IsUndefined() || fileObj.IsNull() {
		return nil, ErrInvalidPromise
	}

	arrayBufferPromise := fileObj.Call("arrayBuffer")
	buffer, err := awaitPromiseSync(arrayBufferPromise)
	if err != nil {
		return nil, err
	}

	return convertArrayBufferToBytes(buffer), nil
}

// readFileFromEntryAsync asynchronously reads a file from a directory entry
func readFileFromEntryAsync(entry js.Value, callback func([]byte, error)) {
	if entry.IsUndefined() || entry.IsNull() {
		callback(nil, ErrInvalidPromise)
		return
	}

	filePromise := entry.Call("getFile")
	filePromise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
			callback(nil, ErrInvalidPromise)
			return nil
		}

		fileObj := args[0]
		arrayBufferPromise := fileObj.Call("arrayBuffer")
		arrayBufferPromise.Call("then", js.FuncOf(func(this js.Value, args2 []js.Value) interface{} {
			if len(args2) > 0 && !args2[0].IsUndefined() && !args2[0].IsNull() {
				buffer := args2[0]
				data := convertArrayBufferToBytes(buffer)
				callback(data, nil)
			} else {
				callback(nil, ErrInvalidPromise)
			}
			return nil
		}))
		return nil
	}))
}

// ReadSpecificFile reads a specific file by path from the folder
// Only works with File System Access API
func (ff *FolderFiles) ReadSpecificFile(path string) ([]byte, error) {
	if ff.DirHandle.IsUndefined() || ff.DirHandle.IsNull() {
		return nil, ErrNotAPromise
	}

	// Split path into components
	parts := strings.Split(path, "/")

	// Navigate to the file
	currentHandle := ff.DirHandle
	for i, part := range parts {
		if i == len(parts)-1 {
			// This is the file
			filePromise := currentHandle.Call("getFileHandle", part)
			fileHandle, err := awaitPromiseSync(filePromise)
			if err != nil {
				return nil, err
			}

			// Get the file object
			getFilePromise := fileHandle.Call("getFile")
			fileObj, err := awaitPromiseSync(getFilePromise)
			if err != nil {
				return nil, err
			}

			// Read as ArrayBuffer
			arrayBufferPromise := fileObj.Call("arrayBuffer")
			buffer, err := awaitPromiseSync(arrayBufferPromise)
			if err != nil {
				return nil, err
			}

			return convertArrayBufferToBytes(buffer), nil
		} else {
			// Navigate to subdirectory
			dirPromise := currentHandle.Call("getDirectoryHandle", part)
			nextHandle, err := awaitPromiseSync(dirPromise)
			if err != nil {
				return nil, err
			}
			currentHandle = nextHandle
		}
	}

	return nil, ErrInvalidPromise
}

// Error definitions
var (
	ErrNoFilesSelected = &FileError{Message: "no files selected"}
	ErrInvalidPromise  = &FileError{Message: "invalid promise"}
	ErrNotAPromise     = &FileError{Message: "not a promise"}
)

// FileError represents a file API error
type FileError struct {
	Message string
}

func (e *FileError) Error() string {
	return e.Message
}

// JSError wraps a JavaScript error value
type JSError struct {
	Value js.Value
}

func (e *JSError) Error() string {
	if e.Value.IsUndefined() || e.Value.IsNull() {
		return "JavaScript error"
	}

	if msg := e.Value.Get("message"); msg.Truthy() {
		return msg.String()
	}

	return e.Value.String()
}

// readInputFiles reads files from input element's files property
func readInputFiles(filesList js.Value) (*FolderFiles, error) {
	files := &FolderFiles{
		Files: make(map[string][]byte),
	}

	length := filesList.Get("length").Int()
	for i := 0; i < length; i++ {
		file := filesList.Call("item", i)
		if file.IsUndefined() || file.IsNull() {
			continue
		}

		name := file.Get("name").String()

		// Read file as ArrayBuffer
		arrayBufferPromise := file.Call("arrayBuffer")
		buffer, err := awaitPromiseSync(arrayBufferPromise)
		if err != nil {
			continue
		}

		// Convert to Go bytes
		data := convertArrayBufferToBytes(buffer)
		files.Files[name] = data
	}

	return files, nil
}

// convertArrayBufferToBytes converts JS ArrayBuffer to Go byte slice
func convertArrayBufferToBytes(buffer js.Value) []byte {
	if buffer.IsUndefined() || buffer.IsNull() {
		return nil
	}

	uint8Array := js.Global().Get("Uint8Array").New(buffer)
	length := uint8Array.Get("length").Int()
	if length == 0 {
		return nil
	}

	data := make([]byte, length)
	js.CopyBytesToGo(data, uint8Array)
	return data
}
