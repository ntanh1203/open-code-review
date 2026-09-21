// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/alibaba/open-code-review/internal/dashboard"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "dashboard.db", "SQLite database path")
	ocrBin := flag.String("ocr-bin", "", "path to ocr binary (default: ocr from PATH)")
	webhookSecret := flag.String("webhook-secret", "", "GitLab webhook secret token")
	flag.Parse()

	// Open database.
	db, err := dashboard.OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Start worker.
	worker := dashboard.NewWorker(db, *ocrBin, 32)
	worker.Start()

	// Create server.
	srv, err := dashboard.NewServer(db, worker, *webhookSecret)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	// Graceful shutdown.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		log.Println("shutting down...")
		worker.Stop()
		os.Exit(0)
	}()

	log.Printf("OCR Dashboard listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
