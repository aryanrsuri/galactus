package main

import (
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Option struct {
	message string
	task    string
	labels  []string
}

type Span struct {
	span_id string
	task_id string
	open int64
	close int64
	comment string
}

func get_hash(comment string) string {
	sum := sha256.Sum256([]byte(comment))
	return fmt.Sprintf("%x", sum)
}

func get_hash_from_prefix(prefix string) string {
	_ = prefix
	return "Not done yet"
}

func run(db *sql.DB) error {
	input := os.Args[1:]
	flag.Parse()
	if len(input) == 0 {
		help := `
 galactus is a time span tracker

 open a span: 'open -task "<task comment>" <space separated labels>'
 close a span: 'close "<span comment>"
 show status of any current span: 'status'
		`

		status(nil, Option{message: help})
	}
	switch input[0] {
	case "open":
		if len(input) < 2 {
			return fmt.Errorf("opening span requires `open -task <arguement>` and optional space separated labels\n")
		}
		if input[1] != "-task" {
			return fmt.Errorf("open requires a `-task` argument\n")
		}
		var option Option
		option.task = input[2]
		option.labels = input[3:]
		span, err := open(db, option)
		if err != nil {
			return err
		}
		status(span, Option{message: "New span opened", task: option.task, labels: option.labels})
		return nil
	case "close":
		if len(input) < 2 {
			return fmt.Errorf("closing a span requires a comment `close '<comment>'`\n")
		}
		span, err := close(db, input[1])
		if err != nil {
			return err
		}
		status(span, Option{message: "Span closed"})
		return nil
	case "status":
		span := get_span(db)
		var message string
		if span == nil {
			message = "No span open"
		} else {
			message = "Current span status"
		}
		status(span, Option{message: message})
		return nil
	case "gantt":
		return gantt(db)
	default:
		status(nil, Option{message: fmt.Sprintf("unkown command %s\n", input[0])})
		return nil
	}
}

func get_span(db *sql.DB) *Span {
	var span Span
	if err := db.QueryRow("SELECT span_id, task_id, open FROM spans WHERE close IS NULL LIMIT 1;").Scan(&span.span_id, &span.task_id, &span.open); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		log.Panicf("could not query span table, check error: %v\n", err)
	}
	return &span
}

func open(db *sql.DB, options Option) (*Span, error) {
	if span := get_span(db); span != nil {
		options.message = "A span has already been opened"
		status(span, options)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, label := range options.labels {
		if _, err := tx.Exec("INSERT OR IGNORE INTO labels (label_id, comment) VALUES (?, ?);", get_hash(label), label); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec("INSERT OR IGNORE INTO tasks (task_id, comment) VALUES (?, ?);", get_hash(options.task), options.task); err != nil {
		return nil, err
	}

	open := time.Now().Unix()
	span_id := get_hash(strconv.FormatInt(open, 10) + options.task)

	if _, err := tx.Exec("INSERT INTO spans (span_id, task_id, open) VALUES (?, ?, ?);", span_id, get_hash(options.task), open); err != nil {
		return nil, err
	}

	for _, label := range options.labels {
		if _, err := tx.Exec("INSERT INTO span_labels (span_id, label_id) VALUES (?, ?);", span_id, get_hash(label)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// I could construct the Span object declaratively
	// but this seems more correct.
	span := get_span(db)
	return span, nil
}

func close(db *sql.DB, comment string) (*Span, error) {
	span := get_span(db)
	if span == nil {
		status(span, Option{message: "No span open"})
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	close := time.Now().Unix()
	if _, err := tx.Exec("UPDATE spans SET close = ?, comment = ? WHERE span_id = ?;", close, comment, span.span_id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	span.close = close 
	span.comment = comment
	return span, nil
}

// Will need to extend this for
// a) List of spans (history)
// b) Nil span (or, better)
func status(span *Span, option Option) {
	if span == nil {
		fmt.Printf("\n %s\n\n", option.message)
		os.Exit(0)
	}
	var start string = time.Unix(span.open, 0).UTC().String()
	var close string
	if span.close == 0 {
		close = ""
	} else {
		close = time.Unix(span.close, 0).UTC().String()
	}
	comment := fmt.Sprintf("\n %s\n\n opened: %s\n closed: %s\n span: %s\n comment: %s\n\n task: %s\n", option.message, start, close, span.span_id, span.comment, span.task_id)
	if option.task != "" {
		comment = comment + fmt.Sprintf(" task comment: %s\n", option.task)
	}

	if len(option.labels) > 0 {
		comment = comment + " labels:"
		for _, label := range option.labels {
			comment = comment + fmt.Sprintf(" %s,", label)
		}
		comment = comment[:len(comment)-1]
	}
	fmt.Println(comment)
	os.Exit(0)
}

func gantt(db *sql.DB) error {
	return nil
}

