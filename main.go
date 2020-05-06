package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/irekovic/go-sync/metadata"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/fsnotify/fsnotify"
	_ "github.com/mattn/go-sqlite3"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"
)

type fileChange struct {
	src, name string
	deletion  bool
}

func main() {
	// UNIX Time is faster and smaller than most timestamps
	// If you set zerolog.TimeFieldFormat to an empty string,
	// logs will write with UNIX time
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.InfoLevel)

	bucketURL := flag.String("url", "", "Bucket URL")
	folderToMonitor := flag.String("f", "", "Folder to monitor")

	flag.Parse()

	if *bucketURL == "" || *folderToMonitor == "" {
		flag.Usage()
		os.Exit(-1)
	}

	rootPath, err := filepath.Abs(*folderToMonitor)
	if err != nil {
		flag.Usage()
		os.Exit(-2)
	}

	// channel to communicate file changes
	changes := make(chan fileChange, 1000)
	// os watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Err(err).Msg("unable to create os watcher")
	}

	// "thread" that is listening on os notifications and
	// translates them into fileChange events
	go func() {
		for fc := range watcher.Events {
			// fmt.Println(fc)
			switch fc.Name {
			case "", ".", "..":
			default:
				changes <- fileChange{"watcher", fc.Name, fc.Op&fsnotify.Remove > 0}
				log.Info().Msgf("OS NOTIFICATION: %+v", fc)
			}
		}
	}()

	rootPath = filepath.Clean(rootPath)
	// open metadata store in monitored folder
	repo, err := metadata.Open(rootPath)
	if err != nil {
		log.Panic().Msgf("unable to open metadata store: %s", err.Error())
	}

	// channal for changes that must end up on cloud
	tocloud := make(chan fileChange)

	// MAIN HANDLER LOOP - HANDLING SYNCING
	// this routine (thread) is dealing with metadata and
	go func() {
		for fi := range changes {
			// fmt.Println(fi)

			if fi.deletion {
				log.Printf("deleting file %v", fi)
				repo.Inc()
				repo.Delete(fi.name)
				tocloud <- fi
			} else {
				info, err := os.Stat(fi.name)
				if err != nil {
					log.Printf("unable to open file: %v", err)
					continue
				}
				r, ok := repo.Get(fi.name)
				mdfi := metadata.NewFileInfo(info)
				if !ok || mdfi != r.FileInfo {
					log.Info().Msgf("updating file %v %v", r.FileInfo, mdfi)
					repo.Inc()
					if !ok {
						r.Created = repo.Version()
					}
					r.Modified = repo.Version()
					r.FileInfo = mdfi
					repo.Put(fi.name, r)
					tocloud <- fi
				}
			}
		}
	}()

	// handle interactions with destination - uploads and deletes against cloud
	go func() {
		b, err := blob.OpenBucket(context.Background(), *bucketURL)
		if err != nil {
			// panic because there is no point in continuing to work
			// if we can't access cloud
			log.Panic().Msg(err.Error())
		}
		defer b.Close()

		for fi := range tocloud {
			fname, _ := filepath.Rel(rootPath, fi.name)
			log.Info().Msgf("Sending to cloud: %v", fi)
			if fi.deletion {
				log.Info().Msgf("Deleting file: %v", fi)
				b.Delete(context.Background(), fname)
			} else {
				copyToBlob(b, fname, fi)
			}
		}
	}()

	// walk function - it is using trackingSet to track for deletes in between runs.
	trackingSet := make(map[string]struct{})
	walkFilepath := func() {
		log.Info().Msg("start walking")
		defer log.Info().Msg("done walking")

		visited := make(map[string]struct{})
		filepath.Walk(rootPath, func(n string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				fname, _ := filepath.Rel(rootPath, fi.Name())
				fmt.Printf("%v, %v, %v\n", fname, rootPath, fi.Name())
				if fname == ".sync" {
					return filepath.SkipDir
				}
				return nil
			}

			changes <- fileChange{"walk", n, false}
			delete(trackingSet, n)
			visited[n] = struct{}{}
			changes <- fileChange{"walk", n, false}
			return nil
		})
		for k := range trackingSet {
			changes <- fileChange{"afterwalk", k, true}
		}
		trackingSet = visited

	}

	ticker := time.NewTicker(time.Minute * 1)
	defer ticker.Stop()

	// Routine running walk at start, and than at every timer pulse (1minute currentl)
	go func() {
		walkFilepath()
		for range ticker.C {
			walkFilepath()
		}
	}()

	// Block main routine untill we receive one of the signals:
	//SIGHUP, SIGINT, or SIGTERM
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	<-signals
}

func copyToBlob(b *blob.Bucket, fname string, fi fileChange) {
	writeCtx, cancelWrite := context.WithCancel(context.Background())
	defer cancelWrite()

	w, err := b.NewWriter(writeCtx, fname, nil)
	if err != nil {
		log.Error().Msgf("Unable to write file:%v", fname)
		return
	}
	fle, err := os.Open(fi.name)
	if err != nil {
		log.Error().Msgf("Unable to open file for reading: %v", fi.name)
		return
	}
	defer fle.Close()

	_, err = io.Copy(w, fle)
	if err != nil {
		log.Error().Msgf("Error while copying bytes: %v %v", fname, err)
		return
	}

	if err := w.Close(); err != nil {
		log.Error().Msgf("Unable to fsync files: %v %v", fname, err)
		return
	}

}
