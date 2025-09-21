package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/user"

	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	user string
	root string
	config string
	shards uint64
}

const SCHEMA = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;


CREATE TABLE IF NOT EXISTS labels(
  label_id  TEXT PRIMARY KEY,
  comment   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks(
  task_id   TEXT PRIMARY KEY,
  comment   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS spans(
	span_id 	TEXT PRIMARY KEY,
  task_id   TEXT NOT NULL REFERENCES tasks(task_id),
  open			INTEGER NOT NULL,
  close			INTEGER NULL,
  comment   TEXT NULL
);

CREATE TABLE IF NOT EXISTS span_labels (
  span_id   TEXT NOT NULL,
  label_id  TEXT NOT NULL,
  PRIMARY KEY(span_id, label_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_span_labels ON span_labels(span_id,label_id);
CREATE INDEX IF NOT EXISTS idx_spans_close_null ON spans(close) WHERE close IS NULL;
CREATE INDEX IF NOT EXISTS idx_spans_task_open ON spans(task_id, open);
CREATE INDEX IF NOT EXISTS idx_spans_open_desc ON spans(open DESC);
`

func create_root(config Config) error {
	if err := os.Mkdir(config.root, os.ModePerm); err != nil {
		return err
	}
	data := fmt.Sprintf("USER %s\nROOT %s\nCONFIG %s\nSHARDS %d", config.user, config.root, config.config, config.shards)
	if err := os.WriteFile(config.config, []byte(data), os.ModePerm); err != nil {
		return err
	}
	return nil
}

func ensure_root(config Config) error {
	if _, err := os.Stat(config.root); err != nil {
		if os.IsNotExist(err) {
			log.Printf("generating galactus at: %s\n", config.root)
			return create_root(config)
		}
		return err
	}
	return nil
}

func get_connection(config Config) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s/%d.db", config.root, config.shards))
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func main() {
	user, _:= user.Current()
	home, _ := os.UserHomeDir()
	config := Config{
		user: user.Username,
		root: home + "/.galactus",
		config: home + "/.galactus" + "/config.json",
		shards: 1,
	}
	if err := ensure_root(config); err != nil {
		log.Panicf("could not create root folder, check error: %v\n", err)
	}

	db, err := get_connection(config)
	if err != nil {
		log.Panicf("could not open connection to database, check error: %v\n", err)
	}

	if _, err := db.Exec(SCHEMA); err != nil {
		log.Panicf("could not generate schema tables, check error: %v\n", err)
	}

	if err := run(db); err != nil {
		log.Fatal(err)
	}
}
