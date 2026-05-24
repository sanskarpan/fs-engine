package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourname/fs-engine/api"
	"github.com/yourname/fs-engine/internal/disk"
	fstype "github.com/yourname/fs-engine/internal/fs"
)

func main() {
	var (
		addr    = flag.String("addr", ":8080", "HTTP server address")
		imgPath = flag.String("img", "testdata/disk.img", "Disk image path")
		format  = flag.Bool("format", false, "Format the filesystem before mounting")
	)
	flag.Parse()

	fmt.Println("filesystem engine starting")
	log.Printf("disk image: %s", *imgPath)
	log.Printf("listening on: %s", *addr)

	// Open or create the disk image
	dev, err := disk.NewBlockDevice(*imgPath, fstype.DiskSize)
	if err != nil {
		log.Fatalf("open disk: %v", err)
	}

	var filesystem *fstype.Filesystem

	if *format {
		log.Println("formatting filesystem...")
		filesystem, err = fstype.NewFormat(dev)
		if err != nil {
			log.Fatalf("format: %v", err)
		}
		log.Println("filesystem formatted and mounted")
	} else {
		// Try to mount; format if not yet initialised
		filesystem, err = fstype.Mount(dev)
		if err != nil {
			log.Printf("mount failed (%v); formatting...", err)
			filesystem, err = fstype.NewFormat(dev)
			if err != nil {
				log.Fatalf("format: %v", err)
			}
			log.Println("filesystem formatted and mounted")
		} else {
			log.Println("filesystem mounted")
		}
	}

	defer func() {
		if err := filesystem.Unmount(); err != nil {
			log.Printf("unmount error: %v", err)
		}
		if err := dev.Close(); err != nil {
			log.Printf("close dev error: %v", err)
		}
		log.Println("filesystem unmounted")
	}()

	// Verify filesystem health
	if err := filesystem.Fsck(); err != nil {
		log.Printf("fsck warning: %v", err)
	}

	total, used, free := filesystem.Df()
	log.Printf("disk usage: total=%dMB used=%dMB free=%dMB",
		total/(1024*1024), used/(1024*1024), free/(1024*1024))

	// Set up API server
	srv := api.NewServer(filesystem)

	httpServer := &http.Server{
		Addr:         *addr,
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		log.Printf("HTTP server listening on %s", *addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
