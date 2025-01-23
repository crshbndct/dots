package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

func main() {
	// Prompt for dry-run flag
	var dryRunInput string
	fmt.Print("Dry-run (yes/no): ")
	fmt.Scanln(&dryRunInput)
	dryRun := strings.ToLower(dryRunInput) == "yes"

	// Prompt for source directory
	var sourceDir string
	fmt.Print("Enter source directory: ")
	fmt.Scanln(&sourceDir)

	// Prompt for target directory
	var targetDir string
	fmt.Print("Enter target directory: ")
	fmt.Scanln(&targetDir)

	// Check if directories exist
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		fmt.Println("Error: Source directory does not exist.")
		return
	}

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		fmt.Println("Error: Target directory does not exist.")
		return
	}

	// Log the user's input
	fmt.Printf("Source Directory: %s\n", sourceDir)
	fmt.Printf("Target Directory: %s\n", targetDir)
	fmt.Printf("Dry Run: %v\n", dryRun)

	// Process the files
	err := processFiles(sourceDir, targetDir, dryRun)
	if err != nil {
		fmt.Println("Error processing files:", err)
	}
}
// processFiles processes files in the source directory
func processFiles(sourceDir, targetDir string, dryRun bool) error {
	fileHashes := make(map[string]string) // Map to track seen files based on their hash
	timestampCounter := make(map[string]int) // Map to track file counts for duplicate timestamps

	// Walk through the source directory
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Normalize extension to lowercase
		ext := strings.ToLower(filepath.Ext(path))

		// Check if it's an image or video file
		if isImageFile(ext) || isVideoFile(ext) {
			// Calculate the SHA-256 hash of the file
			hash, err := calculateFileHash(path)
			if err != nil {
				return err
			}

			// Skip duplicate files based on hash
			if existingPath, exists := fileHashes[hash]; exists {
				fmt.Printf("Duplicate file found: %s (duplicate of %s)\n", path, existingPath)
				if !dryRun {
					err := os.Remove(path)
					if err != nil {
						fmt.Printf("Error deleting file %s: %v\n", path, err)
					} else {
						fmt.Printf("Deleted duplicate file: %s\n", path)
					}
				}
				return nil
			}

			// Add the hash to the map
			fileHashes[hash] = path

			// Get the date for renaming and sorting
			fileDate, err := getFileDate(path)
			if err != nil {
				// Log the error and continue processing
				fmt.Printf("Error extracting date for file %s: %v\n", path, err)
				// Optionally, fall back to current time if the date is missing
				fileDate = time.Now() // Use current time as fallback
			}

			// Define top-level folder paths for images and videos
			var subDir string
			if isImageFile(ext) {
				// For images, use the 'images' top-level folder
				subDir = filepath.Join(targetDir, "images", fileDate.Format("2006"), fileDate.Format("01"))
			} else if isVideoFile(ext) {
				// For videos, use the 'videos' top-level folder
				subDir = filepath.Join(targetDir, "videos", fileDate.Format("2006"), fileDate.Format("01"))
			}

			// Create the target directory path (year/month)
			err = os.MkdirAll(subDir, os.ModePerm)
			if err != nil {
				return err
			}

			// Create a timestamp string for naming
			timestampStr := fileDate.Format("02-01-2006-15-04-05")

			// Check if the timestamp already exists in the map
			counter := 0
			if count, exists := timestampCounter[timestampStr]; exists {
				counter = count + 1
			}
			// Increment the counter for the timestamp
			timestampCounter[timestampStr] = counter

			// If the counter is greater than 0, add the counter to the filename
			var newFileName string
			if counter > 0 {
				// Add counter with 2 leading zeros
				newFileName = fmt.Sprintf("%s-%02d%s", timestampStr, counter, ext)
			} else {
				// No counter needed, just use the timestamp
				newFileName = fmt.Sprintf("%s%s", timestampStr, ext)
			}

			// Generate the full path for the new file
			newPath := filepath.Join(subDir, newFileName)

			// If not dry run, move and rename the file
			if !dryRun {
				err := moveFile(path, newPath)
				if err != nil {
					return err
				}
				fmt.Printf("Moved and renamed: %s -> %s\n", path, newPath)
			} else {
				fmt.Printf("Dry-run: File would be moved and renamed: %s -> %s\n", path, newPath)
			}
		}

		return nil
	})
}


// isImageFile checks if the file extension is a known image extension
func isImageFile(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".webp":
		return true
	default:
		return false
	}
}

// isVideoFile checks if the file extension is a known video extension
func isVideoFile(ext string) bool {
	switch ext {
	case ".mp4", ".avi", ".mov", ".mkv", ".vob", ".flv", ".wmv", ".webm",".mpg":
		return true
	default:
		return false
	}
}

// calculateFileHash computes the SHA-256 hash of a file
func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	_, err = io.Copy(hasher, file)
	if err != nil {
		return "", err
	}

	hash := hasher.Sum(nil)
	return hex.EncodeToString(hash), nil
}

// getFileDate attempts to extract the date from an image or video file
// First tries EXIF (for images), then creation date for videos or fallback
func getFileDate(filePath string) (time.Time, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	var fileDate time.Time
	var err error

	if isImageFile(ext) {
		// Try to read EXIF data for images
		fileDate, err = getImageDate(filePath)
		if err != nil {
			return time.Time{}, fmt.Errorf("error extracting date from image %s: %v", filePath, err)
		}
	} else if isVideoFile(ext) {
		// Try to read video metadata or fallback to file creation date
		fileDate, err = getVideoDate(filePath)
		if err != nil {
			return time.Time{}, fmt.Errorf("error extracting date from video %s: %v", filePath, err)
		}
	} else {
		// Fallback to file creation date if no EXIF or metadata is available
		fileDate, err = getFileCreationDate(filePath)
		if err != nil {
			return time.Time{}, fmt.Errorf("error getting file creation date for %s: %v", filePath, err)
		}
	}

	return fileDate, nil
}
// getImageDate reads EXIF data from an image and returns the date taken
func getImageDate(filePath string) (time.Time, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, err
	}
	defer file.Close()

	x, err := exif.Decode(file)
	if err != nil {
		// If EXIF decoding fails (e.g., EOF or invalid EXIF data), log the error and fallback to creation date
		return time.Time{}, fmt.Errorf("error extracting EXIF data from image %s: %v", filePath, err)
	}

	date, err := x.DateTime()
	if err != nil {
		// If there's an issue with the date extraction, fallback to creation date
		return time.Time{}, fmt.Errorf("error extracting date from EXIF data for image %s: %v", filePath, err)
	}

	return date, nil
}


// getVideoDate uses file creation time or fallback
func getVideoDate(filePath string) (time.Time, error) {
	// Here, you could use libraries like `ffmpeg` to extract video metadata.
	// For simplicity, we'll fall back to creation date
	return getFileCreationDate(filePath)
}

// getFileCreationDate returns the file's creation date
func getFileCreationDate(filePath string) (time.Time, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return time.Time{}, err
	}
	// Use the file's creation time (on most systems, this is available)
	return info.ModTime(), nil
}

// moveFile moves a file from source to target location
func moveFile(src, dest string) error {
	// Ensure the target directory exists
	err := os.MkdirAll(filepath.Dir(dest), os.ModePerm)
	if err != nil {
		return err
	}

	// Move the file
	err = os.Rename(src, dest)
	if err != nil {
		// If renaming fails (e.g., across file systems), try copying
		err = copyFile(src, dest)
		if err != nil {
			return err
		}
		// After copying, delete the source file
		err = os.Remove(src)
		if err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a file from src to dest
func copyFile(src, dest string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}
