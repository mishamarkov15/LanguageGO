package main

import (
	"log"
	"net/http"
	"time"

	"hw-5/gateway/internal/api"
)

func main() {
	server := &http.Server{
		Addr:              ":8080",
		Handler:           api.NewRouter(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Gateway listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
