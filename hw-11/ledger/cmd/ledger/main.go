// Команда проверяет подключения библиотеки Ledger к PostgreSQL и Redis.
package main

import (
	"context"
	"log"
	"time"

	"hw-11/ledger"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, closeLedger, err := ledger.New(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := closeLedger(); err != nil {
		log.Fatal(err)
	}
	log.Print("Ledger: подключения проверены")
}
