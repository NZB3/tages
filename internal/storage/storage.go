package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	pb "github.com/nzb3/tages/pkg/pb"
)

type storage struct {
	mu        sync.RWMutex
	directory string
	files     map[string]*pb.FileMetadata
}

func NewStorage(directory string) *storage {
	return &storage{
		directory: directory,
		files:     make(map[string]*pb.FileMetadata),
	}
}

func (s *storage) SaveFile(filename string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	filepath := filepath.Join(s.directory, id)

	err := ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		return "", err
	}

	now := time.Now().Unix()
	metadata := &pb.FileMetadata{
		Id:        id,
		Filename:  filename,
		Size:      int64(len(data)),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.files[id] = metadata

	return id, nil
}

func (s *storage) GetFile(id string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filepath := filepath.Join(s.directory, id)
	return os.ReadFile(filepath)
}

func (s *storage) ListFiles(pageSize int32, pageNumber int32) ([]*pb.FileMetadata, int32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start := int(pageSize) * (int(pageNumber) - 1)
	end := start + int(pageSize)
	if end > len(s.files) {
		end = len(s.files)
	}

	files := make([]*pb.FileMetadata, 0, end-start)
	i := 0
	for _, file := range s.files {
		if i >= start && i < end {
			files = append(files, file)
		}
		i++
		if i >= end {
			break
		}
	}

	return files, int32(len(s.files)), nil
}
