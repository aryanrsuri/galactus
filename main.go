package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"os"
)

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
  begin     INTEGER NOT NULL,
  end       INTEGER NULL,
  comment   TEXT NULL
);

CREATE TABLE IF NOT EXISTS span_labels (
  span_id   TEXT NOT NULL,
  label_id  TEXT NOT NULL,
  PRIMARY KEY(span_id, label_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_span_labels ON span_labels(span_id,label_id);
CREATE INDEX IF NOT EXISTS idx_spans_end_null ON spans(end) WHERE end IS NULL;
CREATE INDEX IF NOT EXISTS idx_spans_task_begin ON spans(task_id, begin);
CREATE INDEX IF NOT EXISTS idx_spans_begin_desc ON spans(begin DESC);
`

func create_root(root string) error {
	if err := os.Mkdir(root, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func ensure_root(root string) error {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			log.Printf("generating galactus at: %s\n", root)
			return create_root(root)
		}
		return err
	}
	return nil
}

func get_connection(file_path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", file_path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func main() {
	home, _ := os.UserHomeDir()
	root := home + "/.galactus"
	if err := ensure_root(root); err != nil {
		log.Panicf("could not create root folder, check error: %v\n", err)
	}

	db, err := get_connection(root + "/main.db")
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
