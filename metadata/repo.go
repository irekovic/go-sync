package metadata

import (
	"database/sql"
	"fmt"
)

// Repo for working with metadata
type Repo struct {
	Folder  string
	db      *sql.DB
	version SyncID
	tx      *sql.Tx
}

// Inc increments version of repository
func (f *Repo) Inc() SyncID {
	f.d().Exec(`update replicas set version=version+1 where id=0`)

	v := f.version
	f.d().QueryRow("select id, version from replicas where id=0").
		Scan(&v.ReplicaID, &v.Version)
	f.version = v
	return f.version
}

func (f *Repo) Tx(fn func()) error {
	f.tx, _ = f.db.Begin()
	fn()
	err := f.tx.Commit()
	f.tx = nil
	return err
}

// Version returns current version of repository
func (f *Repo) Version() SyncID {
	return f.version
}

// Close will dispose resource held by Repo instance
func (f *Repo) Close() error {
	return f.db.Close()
}

// Get will return record from database
func (f *Repo) Get(s string) (Record, bool) {
	var r Record
	if err := f.d().QueryRow(
		`select thomb, size, mode, modified_at, 
		created_replica, created_version, modified_replica, modified_version 
		from items where id=$1`, s).Scan(
		&r.FileInfo.Thomb, &r.FileInfo.Size, &r.FileInfo.Mode, &r.FileInfo.Modified, &r.Created.ReplicaID, &r.Created.Version, &r.Modified.ReplicaID, &r.Modified.Version,
	); err != nil {
		if err == sql.ErrNoRows {
			return r, false
		}
		return Record{}, false
	}
	r.FileInfo.Modified = r.FileInfo.Modified.UTC()
	return r, true
}

func (f *Repo) Delete(s string) error {
	_, err := f.d().Exec(
		`update items set tick=$1, thomb=true where id=$2`,
		f.version.Version,
		s,
	)
	return err
}

//Touch will mark file item as present
func (f *Repo) Touch(s string) error {
	_, err := f.d().Exec(
		`update items set tick=$1,thomb=false where id=$2`,
		f.version.Version,
		s,
	)
	return err
}

// Mark will flag all removed files.
func (f *Repo) Mark(fn func(string)) error {
	_, err := f.d().Exec(
		`update items set thomb=true, tick=$1 where thomb=false and tick<$1`,
		f.version.Version,
		f.version.Version,
	)
	if err != nil {
		fmt.Println(err)
		return err
	}
	rows, err := f.d().Query(
		`select id
		from items 
		where thomb=true and tick=$1;`,
		f.version.Version)
	if err != nil {
		fmt.Println(err)
		return err
	}

	defer rows.Close()
loop:
	for rows.Next() {
		var relative string
		if err := rows.Scan(&relative); err != nil {
			break loop
		}
		fmt.Println(err)
		fn(relative)
	}
	fmt.Println("Done marking deleted files")
	return rows.Err()

}

//Put allow client to store value for a key
func (f *Repo) Put(s string, r Record) error {
	_, err := f.d().Exec(
		`insert into items (id, tick, thomb, size, mode, modified_at,created_replica,created_version,modified_replica,modified_version) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		on conflict(id) do update set 
			tick=excluded.tick,
			thomb=false,
			size=excluded.size, 
			mode=excluded.mode,
			modified_at=excluded.modified_at,
			created_replica=excluded.created_replica,
			created_version=excluded.created_version,
			modified_replica=excluded.modified_replica,
			modified_version=excluded.modified_version;`,
		s,
		f.version.Version,
		false,
		r.FileInfo.Size,
		r.FileInfo.Mode,
		r.FileInfo.Modified,
		r.Created.ReplicaID,
		r.Created.Version,
		r.Modified.ReplicaID,
		r.Modified.Version,
	)
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

func (f *Repo) d() data {
	if f.tx != nil {
		return f.tx
	}
	return f.db
}

type data interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Query(query string, args ...interface{}) (*sql.Rows, error)
}
