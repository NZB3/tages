package main

import (
	"log"

	fileserver "github.com/nzb3/tages/internal/server"
	filestorage "github.com/nzb3/tages/internal/storage"
)

func main() {
	storage := filestorage.NewStorage("./storage")
	server := fileserver.NewServer(storage)

	err := server.Serve()
	if err != nil {
		log.Fatal(err)
	}
}
