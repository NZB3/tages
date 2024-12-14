package server

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/nzb3/tages/pkg/pb"
)

type mockStorage struct {
	mock.Mock
}

func (m *mockStorage) SaveFile(filename string, data []byte) (string, error) {
	args := m.Called(filename, data)
	return args.String(0), args.Error(1)
}

func (m *mockStorage) GetFile(id string) ([]byte, error) {
	args := m.Called(id)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockStorage) ListFiles(pageSize int32, pageNumber int32) ([]*pb.FileMetadata, int32, error) {
	args := m.Called(pageSize, pageNumber)
	return args.Get(0).([]*pb.FileMetadata), args.Get(1).(int32), args.Error(2)
}

func setupTest(t *testing.T) (pb.FileServiceClient, *mockStorage, func()) {
	listener := bufconn.Listen(1024 * 1024)
	ms := new(mockStorage)
	server := NewServer(ms)

	s := grpc.NewServer()
	pb.RegisterFileServiceServer(s, server)

	go func() {
		if err := s.Serve(listener); err != nil {
			t.Errorf("Server exited with error: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	client := pb.NewFileServiceClient(conn)

	cleanup := func() {
		listener.Close()
		s.Stop()
		conn.Close()
	}

	return client, ms, cleanup
}

func TestUploadFile(t *testing.T) {
	client, ms, cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name          string
		fileInfo      *pb.FileInfo
		chunks        [][]byte
		expectedError codes.Code
		mockSetup     func(*mockStorage)
	}{
		{
			name: "successful upload",
			fileInfo: &pb.FileInfo{
				Filename:    "test.jpg",
				ContentType: "image/jpeg",
			},
			chunks: [][]byte{[]byte("chunk1"), []byte("chunk2")},
			mockSetup: func(*mockStorage) {
				ms.On("SaveFile", "test.jpg", []byte("chunk1chunk2")).
					Return("123", nil)
			},
			expectedError: codes.OK,
		},
		{
			name:          "missing file info",
			chunks:        [][]byte{[]byte("chunk1")},
			expectedError: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockSetup != nil {
				tt.mockSetup(ms)
				defer ms.AssertExpectations(t)
			}

			stream, err := client.UploadFile(context.Background())
			require.NoError(t, err)

			if tt.fileInfo != nil {
				err = stream.Send(&pb.UploadFileRequest{
					Data: &pb.UploadFileRequest_Info{
						Info: tt.fileInfo,
					},
				})
				require.NoError(t, err)
			}

			for _, chunk := range tt.chunks {
				err = stream.Send(&pb.UploadFileRequest{
					Data: &pb.UploadFileRequest_ChunkData{
						ChunkData: chunk,
					},
				})
				require.NoError(t, err)
			}

			response, err := stream.CloseAndRecv()
			if tt.expectedError != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tt.expectedError, st.Code())
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, response.Id)
			}
		})
	}
}

func TestDownloadFile(t *testing.T) {
	client, ms, cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name          string
		fileID        string
		mockData      []byte
		expectedError codes.Code
		mockSetup     func(*mockStorage)
	}{
		{
			name:     "successful download",
			fileID:   "123",
			mockData: []byte("test data"),
			mockSetup: func(*mockStorage) {
				ms.On("GetFile", "123").Return([]byte("test data"), nil)
			},
			expectedError: codes.OK,
		},
		{
			name:   "file not found",
			fileID: "456",
			mockSetup: func(*mockStorage) {
				ms.On("GetFile", "456").Return([]byte{}, status.Error(codes.NotFound, ""))
			},
			expectedError: codes.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockSetup != nil {
				tt.mockSetup(ms)
				defer ms.AssertExpectations(t)
			}

			stream, err := client.DownloadFile(context.Background(), &pb.DownloadFileRequest{Id: tt.fileID})
			require.NoError(t, err)

			var receivedData []byte
			for {
				resp, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if tt.expectedError != codes.OK {
					st, ok := status.FromError(err)
					require.True(t, ok)
					assert.Equal(t, tt.expectedError, st.Code())
					return
				}
				require.NoError(t, err)
				receivedData = append(receivedData, resp.ChunkData...)
			}

			if tt.expectedError == codes.OK {
				assert.Equal(t, tt.mockData, receivedData)
			}
		})
	}
}

func TestListFiles(t *testing.T) {
	client, ms, cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name          string
		pageSize      int32
		pageNumber    int32
		mockFiles     []*pb.FileMetadata
		totalCount    int32
		expectedError codes.Code
		mockSetup     func(*mockStorage)
	}{
		{
			name:       "successful listing",
			pageSize:   10,
			pageNumber: 1,
			mockFiles: []*pb.FileMetadata{
				{
					Id:        "1",
					Filename:  "test1.jpg",
					Size:      100,
					CreatedAt: time.Now().Unix(),
					UpdatedAt: time.Now().Unix(),
				},
			},
			totalCount: 1,
			mockSetup: func(*mockStorage) {
				ms.On("ListFiles", int32(10), int32(1)).
					Return([]*pb.FileMetadata{{
						Id:        "1",
						Filename:  "test1.jpg",
						Size:      100,
						CreatedAt: time.Now().Unix(),
						UpdatedAt: time.Now().Unix(),
					}}, int32(1), nil)
			},
			expectedError: codes.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockSetup != nil {
				tt.mockSetup(ms)
				defer ms.AssertExpectations(t)
			}

			response, err := client.ListFiles(context.Background(), &pb.ListFilesRequest{
				PageSize:   tt.pageSize,
				PageNumber: tt.pageNumber,
			})

			if tt.expectedError != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tt.expectedError, st.Code())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.totalCount, response.TotalCount)
				assert.Equal(t, len(tt.mockFiles), len(response.Files))
			}
		})
	}
}
