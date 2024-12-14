package storage

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorage_SaveAndGetImageFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "storage_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Initialize storage
	s := NewStorage(tempDir)

	// Create a test image
	img := createTestImage(100, 100)

	// Convert image to bytes
	var buf bytes.Buffer
	err = png.Encode(&buf, img)
	require.NoError(t, err)
	imageBytes := buf.Bytes()

	// Test SaveFile
	filename := "test_image.png"
	id, err := s.SaveFile(filename, imageBytes)
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	// Verify file was saved
	savedFilePath := filepath.Join(tempDir, id)
	_, err = os.Stat(savedFilePath)
	assert.NoError(t, err)

	// Test GetFile
	retrievedBytes, err := s.GetFile(id)
	require.NoError(t, err)
	assert.Equal(t, imageBytes, retrievedBytes)

	// Verify retrieved bytes can be decoded back to an image
	retrievedImg, err := png.Decode(bytes.NewReader(retrievedBytes))
	require.NoError(t, err)
	assert.Equal(t, img.Bounds(), retrievedImg.Bounds())
}

func TestStorage_ListFiles(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "storage_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Initialize storage
	s := NewStorage(tempDir)

	// Save multiple test images
	for i := 0; i < 5; i++ {
		img := createTestImage(50, 50)
		var buf bytes.Buffer
		err = png.Encode(&buf, img)
		require.NoError(t, err)

		_, err := s.SaveFile(fmt.Sprintf("test_image_%d.png", i), buf.Bytes())
		require.NoError(t, err)
	}

	// Test ListFiles
	files, totalCount, err := s.ListFiles(3, 1)
	require.NoError(t, err)
	assert.Equal(t, int32(5), totalCount)
	assert.Len(t, files, 3)

	files, totalCount, err = s.ListFiles(3, 2)
	require.NoError(t, err)
	assert.Equal(t, int32(5), totalCount)
	assert.Len(t, files, 2)
}

func createTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 100, 255})
		}
	}
	return img
}
