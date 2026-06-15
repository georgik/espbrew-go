//go:build !js

package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestCameraPagination(t *testing.T) {
	// Create temporary captures directory
	tmpDir := t.TempDir()
	handler := &CameraHandler{capturesDir: tmpDir}
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test captures
	dates := []string{"2026-06-15", "2026-06-16"}
	for i := 0; i < 50; i++ {
		date := dates[i%2]
		filename := fmt.Sprintf("test-%04d.jpg", i)
		dir := filepath.Join(tmpDir, date)
		os.MkdirAll(dir, 0755)
		path := filepath.Join(dir, filename)
		os.WriteFile(path, []byte("test"), 0644)
	}

	server := httptest.NewServer(router)
	defer server.Close()

	tests := []struct {
		name        string
		page        string
		limit       string
		expectCount int
		expectPage  int
		expectTotal int
		expectPages int
	}{
		{
			name:        "default first page",
			page:        "1",
			limit:       "40",
			expectCount: 40,
			expectPage:  1,
			expectTotal: 50,
			expectPages: 2,
		},
		{
			name:        "second page",
			page:        "2",
			limit:       "40",
			expectCount: 10,
			expectPage:  2,
			expectTotal: 50,
			expectPages: 2,
		},
		{
			name:        "small page size",
			page:        "1",
			limit:       "10",
			expectCount: 10,
			expectPage:  1,
			expectTotal: 50,
			expectPages: 5,
		},
		{
			name:        "page beyond total",
			page:        "10",
			limit:       "40",
			expectCount: 10,
			expectPage:  2,
			expectTotal: 50,
			expectPages: 2,
		},
		{
			name:        "invalid page defaults to 1",
			page:        "invalid",
			limit:       "40",
			expectCount: 40,
			expectPage:  1,
			expectTotal: 50,
			expectPages: 2,
		},
		{
			name:        "invalid limit defaults to 40",
			page:        "1",
			limit:       "invalid",
			expectCount: 40,
			expectPage:  1,
			expectTotal: 50,
			expectPages: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := server.URL + "/api/v1/captures?page=" + tt.page + "&limit=" + tt.limit
			resp, err := http.Get(url)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected status OK, got %v", resp.StatusCode)
			}

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			captures := result["captures"].([]interface{})
			count := len(captures)
			if count != tt.expectCount {
				t.Errorf("Expected %d captures, got %d", tt.expectCount, count)
			}

			page := int(result["page"].(float64))
			if page != tt.expectPage {
				t.Errorf("Expected page %d, got %d", tt.expectPage, page)
			}

			total := int(result["total"].(float64))
			if total != tt.expectTotal {
				t.Errorf("Expected total %d, got %d", tt.expectTotal, total)
			}

			totalPages := int(result["total_pages"].(float64))
			if totalPages != tt.expectPages {
				t.Errorf("Expected total pages %d, got %d", tt.expectPages, totalPages)
			}
		})
	}
}

func TestCameraPaginationEmpty(t *testing.T) {
	// Create empty temporary captures directory
	tmpDir := t.TempDir()
	handler := &CameraHandler{capturesDir: tmpDir}
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/v1/captures?page=1&limit=40")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status OK, got %v", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	capturesVal := result["captures"]
	if capturesVal != nil {
		captures := capturesVal.([]interface{})
		if len(captures) != 0 {
			t.Errorf("Expected 0 captures, got %d", len(captures))
		}
	}

	total := int(result["total"].(float64))
	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	totalPages := int(result["total_pages"].(float64))
	if totalPages != 1 {
		t.Errorf("Expected total_pages 1 for empty result, got %d", totalPages)
	}
}

func TestCameraPaginationParameters(t *testing.T) {
	tmpDir := t.TempDir()
	handler := &CameraHandler{capturesDir: tmpDir}
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create one test capture
	dir := filepath.Join(tmpDir, "2026-06-15")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "test.jpg"), []byte("test"), 0644)

	server := httptest.NewServer(router)
	defer server.Close()

	tests := []struct {
		name        string
		url         string
		expectPage  int
		expectLimit int
	}{
		{
			name:        "no parameters defaults to page 1",
			url:         "/api/v1/captures",
			expectPage:  1,
			expectLimit: 40,
		},
		{
			name:        "page beyond total caps at last page",
			url:         "/api/v1/captures?page=2",
			expectPage:  1,
			expectLimit: 40,
		},
		{
			name:        "only limit parameter",
			url:         "/api/v1/captures?limit=10",
			expectPage:  1,
			expectLimit: 10,
		},
		{
			name:        "negative page defaults to 1",
			url:         "/api/v1/captures?page=-1",
			expectPage:  1,
			expectLimit: 40,
		},
		{
			name:        "zero limit defaults to 40",
			url:         "/api/v1/captures?limit=0",
			expectPage:  1,
			expectLimit: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tt.url)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			page := int(result["page"].(float64))
			if page != tt.expectPage {
				t.Errorf("Expected page %d, got %d", tt.expectPage, page)
			}

			limit := int(result["limit"].(float64))
			if limit != tt.expectLimit {
				t.Errorf("Expected limit %d, got %d", tt.expectLimit, limit)
			}
		})
	}
}

func TestCameraPaginationPathSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	handler := &CameraHandler{capturesDir: tmpDir}
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test captures
	dir := filepath.Join(tmpDir, "2026-06-15")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "test.jpg"), []byte("test"), 0644)

	server := httptest.NewServer(router)
	defer server.Close()

	t.Run("serve capture returns correct content type", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/captures/2026-06-15/test.jpg")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", resp.StatusCode)
		}

		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "image/jpeg") {
			t.Errorf("Expected image/jpeg content type, got %s", contentType)
		}
	})

	t.Run("delete capture requires valid path", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", server.URL+"/api/v1/captures/../../../etc/passwd", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// Should return bad request for path traversal attempts
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected bad request or not found for path traversal, got %v", resp.StatusCode)
		}
	})
}
