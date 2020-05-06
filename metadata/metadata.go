package metadata

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// FileInfo represents information about a file that we have
type FileInfo struct {
	Modified time.Time
	Size     int64
	Mode     int64
	Thomb    bool
}

// NewFileInfo converts i into FileInfo
func NewFileInfo(i os.FileInfo) FileInfo {
	return FileInfo{i.ModTime().UTC(), i.Size(), int64(i.Mode()), false}
}

// SyncID is replicaID+version tuple
type SyncID struct {
	ReplicaID int64
	Version   int64
}

// Inc increments tick and returns new SyncID
func (i SyncID) Inc() SyncID {
	return SyncID{i.ReplicaID, i.Version + 1}
}

// Record contains all metadata that we know about a path
type Record struct {
	FileInfo FileInfo
	Created  SyncID
	Modified SyncID
}

// Open will open metadata file and return connection, replicaID, replica version, and error.
func Open(folder string) (*Repo, error) {
	syncdir := filepath.Join(folder, ".sync")
	os.MkdirAll(syncdir, os.ModePerm)
	file := filepath.Join(folder, ".sync/replica.db")
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return nil, err
	}
	// PRAGMA journal_mode=WAL;
	db.Exec(`

CREATE TABLE IF NOT EXISTS replicas (
  id integer PRIMARY KEY NOT NULL,
  replica_id nvarchar NOT NULL,
  version integer NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_replica_id ON replicas (replica_id ASC);

CREATE TABLE IF NOT EXISTS items (
  id nvarchar PRIMARY KEY NOT NULL,
  size int NOT NULL,
  mode int NOT NULL,
  thomb bool NOT NULL DEFAULT(false),
  tick int NOT NULL,
  modified_at datetime NOT NULL,
  created_replica int NOT NULL,
  created_version int NOT NULL,
  modified_replica int NOT NULL,
  modified_version int NOT NULL,
  FOREIGN KEY (created_replica) REFERENCES replicas (id),
  FOREIGN KEY (modified_replica) REFERENCES replicas (id)
);
CREATE INDEX IF NOT EXISTS idx_mr ON items (modified_replica ASC);

CREATE INDEX IF NOT EXISTS idxmark ON items (tick ASC, thomb ASC);

CREATE INDEX IF NOT EXISTS id_cr ON items (created_replica ASC);
`)
	var ID SyncID

	db.Exec(`insert into replicas (id, replica_id, version) values(0, $1, $2);`, uuid.New().String(), 0)
	db.QueryRow(`select version from replicas where id=0`).Scan(&ID.Version)
	log.Println("SYNCID:", ID)
	return &Repo{Folder: folder, db: db, version: ID}, nil
}
