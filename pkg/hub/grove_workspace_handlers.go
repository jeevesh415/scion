// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hub

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxUploadTotalSize is the maximum total request body size for file uploads (100MB).
const maxUploadTotalSize = 100 * 1024 * 1024

// maxUploadFileSize is the maximum size for a single uploaded file (50MB).
const maxUploadFileSize = 50 * 1024 * 1024

// GroveWorkspaceFile represents a file in a grove workspace.
type GroveWorkspaceFile struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Mode    string    `json:"mode"`
}

// GroveWorkspaceListResponse is the response for listing grove workspace files.
type GroveWorkspaceListResponse struct {
	Files      []GroveWorkspaceFile `json:"files"`
	TotalSize  int64                `json:"totalSize"`
	TotalCount int                  `json:"totalCount"`
}

// GroveWorkspaceUploadResponse is the response for uploading files to a grove workspace.
type GroveWorkspaceUploadResponse struct {
	Files []GroveWorkspaceFile `json:"files"`
}

// handleGroveWorkspace dispatches grove workspace file operations.
// Routes:
//   - GET  (filePath="")  → list files
//   - POST (filePath="")  → upload files
//   - DELETE (filePath!="") → delete file
func (s *Server) handleGroveWorkspace(w http.ResponseWriter, r *http.Request, groveID, filePath string) {
	ctx := r.Context()

	// Look up the grove
	grove, err := s.store.GetGrove(ctx, groveID)
	if err != nil {
		writeErrorFromErr(w, err, "")
		return
	}

	// Only hub-native groves (no git remote) have a managed workspace
	if grove.GitRemote != "" {
		Conflict(w, "Workspace file management is only available for hub-native groves")
		return
	}

	// Resolve workspace path
	workspacePath, err := hubNativeGrovePath(grove.Slug)
	if err != nil {
		InternalError(w)
		return
	}

	switch {
	case r.Method == http.MethodGet && filePath == "":
		s.handleGroveWorkspaceList(w, workspacePath)
	case r.Method == http.MethodPost && filePath == "":
		s.handleGroveWorkspaceUpload(w, r, workspacePath)
	case r.Method == http.MethodDelete && filePath != "":
		s.handleGroveWorkspaceDelete(w, workspacePath, filePath)
	default:
		MethodNotAllowed(w)
	}
}

// handleGroveWorkspaceList lists files in a grove workspace.
func (s *Server) handleGroveWorkspaceList(w http.ResponseWriter, workspacePath string) {
	var files []GroveWorkspaceFile
	var totalSize int64

	err := filepath.WalkDir(workspacePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get the relative path
		relPath, err := filepath.Rel(workspacePath, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Skip the .scion directory
		if relPath == ".scion" || strings.HasPrefix(relPath, ".scion/") || strings.HasPrefix(relPath, ".scion\\") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories (only list files)
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		files = append(files, GroveWorkspaceFile{
			Path:    relPath,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Mode:    info.Mode().String(),
		})
		totalSize += info.Size()

		return nil
	})

	if err != nil {
		// If the workspace directory doesn't exist yet, return empty list
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, GroveWorkspaceListResponse{
				Files:      []GroveWorkspaceFile{},
				TotalSize:  0,
				TotalCount: 0,
			})
			return
		}
		InternalError(w)
		return
	}

	if files == nil {
		files = []GroveWorkspaceFile{}
	}

	writeJSON(w, http.StatusOK, GroveWorkspaceListResponse{
		Files:      files,
		TotalSize:  totalSize,
		TotalCount: len(files),
	})
}

// handleGroveWorkspaceUpload handles file uploads to a grove workspace.
func (s *Server) handleGroveWorkspaceUpload(w http.ResponseWriter, r *http.Request, workspacePath string) {
	// Apply total request body size limit
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadTotalSize)

	// Parse multipart form
	if err := r.ParseMultipartForm(maxUploadTotalSize); err != nil {
		if err.Error() == "http: request body too large" {
			BadRequest(w, "Request body exceeds 100MB limit")
			return
		}
		BadRequest(w, "Invalid multipart form: "+err.Error())
		return
	}

	if r.MultipartForm == nil || len(r.MultipartForm.File) == 0 {
		ValidationError(w, "No files provided", nil)
		return
	}

	var uploaded []GroveWorkspaceFile

	for fieldName, fileHeaders := range r.MultipartForm.File {
		for _, fh := range fileHeaders {
			// The field name is the relative file path
			relPath := fieldName

			// Validate the file path
			if err := validateWorkspaceFilePath(relPath); err != nil {
				BadRequest(w, fmt.Sprintf("Invalid file path %q: %s", relPath, err.Error()))
				return
			}

			// Check per-file size limit
			if fh.Size > maxUploadFileSize {
				BadRequest(w, fmt.Sprintf("File %q exceeds 50MB limit", relPath))
				return
			}

			// Open the uploaded file
			src, err := fh.Open()
			if err != nil {
				InternalError(w)
				return
			}

			// Create parent directories
			destPath := filepath.Join(workspacePath, relPath)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				src.Close()
				InternalError(w)
				return
			}

			// Write file to disk
			dst, err := os.Create(destPath)
			if err != nil {
				src.Close()
				InternalError(w)
				return
			}

			written, err := io.Copy(dst, src)
			src.Close()
			dst.Close()

			if err != nil {
				InternalError(w)
				return
			}

			// Get file info for response
			info, err := os.Stat(destPath)
			if err != nil {
				InternalError(w)
				return
			}

			uploaded = append(uploaded, GroveWorkspaceFile{
				Path:    relPath,
				Size:    written,
				ModTime: info.ModTime(),
				Mode:    info.Mode().String(),
			})
		}
	}

	writeJSON(w, http.StatusOK, GroveWorkspaceUploadResponse{
		Files: uploaded,
	})
}

// handleGroveWorkspaceDelete deletes a file from a grove workspace.
func (s *Server) handleGroveWorkspaceDelete(w http.ResponseWriter, workspacePath, filePath string) {
	// Validate the file path
	if err := validateWorkspaceFilePath(filePath); err != nil {
		BadRequest(w, fmt.Sprintf("Invalid file path %q: %s", filePath, err.Error()))
		return
	}

	fullPath := filepath.Join(workspacePath, filePath)

	// Check file exists
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			NotFound(w, "File")
			return
		}
		InternalError(w)
		return
	}

	// Remove the file
	if err := os.Remove(fullPath); err != nil {
		InternalError(w)
		return
	}

	// Clean up empty parent directories
	cleanEmptyDirs(workspacePath, filepath.Dir(fullPath))

	w.WriteHeader(http.StatusNoContent)
}

// validateWorkspaceFilePath validates that a file path is safe for workspace operations.
// It rejects empty paths, absolute paths, path traversal, and .scion/ prefix.
func validateWorkspaceFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}

	// Reject absolute paths
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed")
	}

	// Clean the path and check for traversal
	cleaned := filepath.Clean(path)
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+"..") {
		return fmt.Errorf("path traversal not allowed")
	}

	// Reject .scion/ prefix
	if cleaned == ".scion" || strings.HasPrefix(cleaned, ".scion/") || strings.HasPrefix(cleaned, ".scion"+string(filepath.Separator)) {
		return fmt.Errorf(".scion directory is reserved")
	}

	return nil
}

// cleanEmptyDirs removes empty directories from targetDir up to (but not including) rootDir.
func cleanEmptyDirs(rootDir, targetDir string) {
	for targetDir != rootDir {
		entries, err := os.ReadDir(targetDir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(targetDir)
		targetDir = filepath.Dir(targetDir)
	}
}
