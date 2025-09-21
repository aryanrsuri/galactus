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

type Options struct {
	message string
	task    string
	labels  []string
	view string
}

type Span struct {
	span_id string
	task_id string
	open int64
	close int64
	comment string

	task_comment string
	labels []string
}



func get_hash(comment string) string {
	sum := sha256.Sum256([]byte(comment))
	return fmt.Sprintf("%x", sum)
}

func get_span(db *sql.DB) *Span {
	var span Span
	if err := db.QueryRow(`
		SELECT spans.span_id, spans.task_id, spans.open, tasks.comment as 'task_comment'
		FROM spans
		JOIN tasks ON spans.task_id = tasks.task_id WHERE spans.close IS NULL LIMIT 1;`).
		Scan(&span.span_id, &span.task_id, &span.open, &span.task_comment); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		log.Panicf("could not query span table, check error: %v\n", err)
	}
	return &span
}

func run(db *sql.DB) error {
	input := os.Args[1:]
	flag.Parse()
	if len(input) == 0 {
		help := `
 galactus is a time span tracker

 open new span: 'open -task "<task comment>" <space separated labels>'
 close current span: 'close "<span comment>"
 show status: 'status'
 show history: 'history'
		`

		status(nil, Options{message: help})
		return nil
	}
	switch input[0] {
	case "open":
		if len(input) < 2 {
			return fmt.Errorf("opening span requires `open -task <arguement>` and optional space separated labels\n")
		}
		if input[1] != "-task" {
			return fmt.Errorf("open requires a `-task` argument\n")
		}
		var options Options
		options.task = input[2]
		options.labels = input[3:]
		span, err := open(db, options)
		if err != nil {
			return err
		}
		if span == nil {
			return nil
		}
		status(span, Options{message: "New span opened", task: options.task, labels: options.labels})
		return nil
	case "close":
		if len(input) < 2 {
			return fmt.Errorf("closing a span requires a comment `close '<comment>'`\n")
		}
		span, err := close(db, input[1])
		if err != nil {
			return err
		}
		if span == nil {
			return nil
		}
		status(span, Options{message: "Span closed"})
		return nil
	case "status":
		span := get_span(db)
		var message string
		if span == nil {
			message = "No span open"
		} else {
			message = "Current span status"
		}
		var view string
		if len(input) == 2 {
			view = input[1]
		}
		status(span, Options{message: message, view: view})
		return nil
	case "history":
		var view string
		if len(input) == 2 {
			view = input[1]
		}
		spans := history(db)
		for _, span := range(spans) {
			status(&span, Options{message: "-", view: view})
		}
		return nil
	case "gantt":
		return gantt(db)
	default:
		status(nil, Options{message: fmt.Sprintf("unkown command %s\n", input[0])})
		return nil
	}
}

func open(db *sql.DB, options Options) (*Span, error) {
	if span := get_span(db); span != nil {
		var option = Options{
			message: "A span has already been opened",
			task: span.task_comment,
			labels: span.labels,
			view: options.view,
		}
		
		status(span, option)
		return nil, nil
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
		status(span, Options{message: "No span open"})
		return nil, nil
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

func history(db *sql.DB) []Span {
	var spans []Span
	rows, err := db.Query(`
		SELECT spans.span_id, spans.task_id, spans.open, spans.close, spans.comment, tasks.comment as 'task_comment'
		FROM spans
		JOIN tasks ON spans.task_id = tasks.task_id;`)

	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var span Span
		rows.Scan(&span.span_id, &span.task_id, &span.open, &span.close, &span.comment, &span.task_comment)
		spans = append(spans, span)
	}
	return spans
}

// Will need to extend this for
// a) List of spans (history)
// b) Nil span (or, better)
func status(span *Span, options Options) {
	if span == nil {
		fmt.Printf(" %s\n\n", options.message)
		return
	}
	var open string = time.Unix(span.open, 0).UTC().String()
	var close string
	if span.close == 0 {
		close = ""
	} else {
		close = time.Unix(span.close, 0).UTC().String()
	}

	var comment string
	if options.view == "condensed" {
		comment = fmt.Sprintf(" S %s %s O %d C %d T %s %s",
		span.span_id[:12], span.comment, span.open, span.close, span.task_id[:12], span.task_comment)
	} else {
		comment = fmt.Sprintf(" %s\n\n span: %s\n opened: %s\n closed: %s\n comment: %s\n\n task: %s\n",
			options.message, span.span_id, open, close, span.comment, span.task_id)
		// FIXME: Remove the need for the options type
		// the fix is to pass in a new struct, when
		// options *was* needed for task / labels
		if options.task != "" {
			comment = comment + fmt.Sprintf(" task comment: %s\n", options.task)
		}
		if span.task_comment != "" {
			comment = comment + fmt.Sprintf(" task comment: %s\n", span.task_comment)
		}

		if len(options.labels) > 0 {
			comment = comment + " labels:"
			for _, label := range options.labels {
				comment = comment + fmt.Sprintf(" %s,", label)
			}
			comment = comment[:len(comment)-1]
		}
	}
	fmt.Println(comment)
}

func gantt(db *sql.DB) error {
	return nil
}
