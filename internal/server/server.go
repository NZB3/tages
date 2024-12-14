package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/nzb3/tages/pkg/pb"
)

const (
	MaxUploadConcurrency   = 10
	MaxDownloadConcurrency = 10
	MaxListConcurrency     = 100
)

type storage interface {
	SaveFile(filename string, data []byte) (string, error)
	GetFile(id string) ([]byte, error)
	ListFiles(pageSize int32, pageNumber int32) ([]*pb.FileMetadata, int32, error)
}

type server struct {
	pb.UnimplementedFileServiceServer
	storage           storage
	uploadSemaphore   chan struct{}
	downloadSemaphore chan struct{}
	listSemaphore     chan struct{}
}

func NewServer(storage storage) *server {
	return &server{
		storage:           storage,
		uploadSemaphore:   make(chan struct{}, MaxUploadConcurrency),
		downloadSemaphore: make(chan struct{}, MaxDownloadConcurrency),
		listSemaphore:     make(chan struct{}, MaxListConcurrency),
	}
}

func (s *server) Serve() error {
	lis, err := net.Listen("tcp", ":9000")
	if err != nil {
		return fmt.Errorf("could not listen on port 9000: %w", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterFileServiceServer(grpcServer, &server{})

	log.Printf("server listening at %v", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}

	return nil
}

func (s *server) UploadFile(stream pb.FileService_UploadFileServer) error {
	select {
	case s.uploadSemaphore <- struct{}{}:
		defer func() { <-s.uploadSemaphore }()
	default:
		return status.Error(codes.ResourceExhausted, "Max concurrent uploads reached")
	}

	req, err := stream.Recv()
	if err != nil {
		return err
	}

	fileInfo := req.GetInfo()
	if fileInfo == nil {
		return status.Error(codes.InvalidArgument, "First message must contain file info")
	}

	var data []byte
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		data = append(data, req.GetChunkData()...)
	}

	id, err := s.storage.SaveFile(fileInfo.Filename, data)
	if err != nil {
		return status.Error(codes.Internal, "Failed to save file")
	}

	return stream.SendAndClose(&pb.UploadFileResponse{
		Id:        id,
		Filename:  fileInfo.Filename,
		Size:      int64(len(data)),
		CreatedAt: time.Now().Unix(),
	})
}

func (s *server) DownloadFile(req *pb.DownloadFileRequest, stream pb.FileService_DownloadFileServer) error {
	select {
	case s.downloadSemaphore <- struct{}{}:
		defer func() { <-s.downloadSemaphore }()
	default:
		return status.Error(codes.ResourceExhausted, "Max concurrent downloads reached")
	}

	data, err := s.storage.GetFile(req.Id)
	if err != nil {
		return status.Error(codes.NotFound, "File not found")
	}

	chunkSize := 1024 * 1024 // 1MB chunks
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		if err := stream.Send(&pb.DownloadFileResponse{ChunkData: data[i:end]}); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	select {
	case s.listSemaphore <- struct{}{}:
		defer func() { <-s.listSemaphore }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "Max concurrent list requests reached")
	}

	files, totalCount, err := s.storage.ListFiles(req.PageSize, req.PageNumber)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to list files")
	}

	return &pb.ListFilesResponse{
		Files:      files,
		TotalCount: totalCount,
	}, nil
}
