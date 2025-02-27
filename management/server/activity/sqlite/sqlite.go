package sqlite

import (
	"database/sql"
	"encoding/json"

	"github.com/netbirdio/netbird/management/server/activity"

	// sqlite driver
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	//eventSinkDB is the default name of the events database
	eventSinkDB      = "events.db"
	createTableQuery = "CREATE TABLE IF NOT EXISTS events " +
		"(id INTEGER PRIMARY KEY AUTOINCREMENT, " +
		"activity INTEGER, " +
		"timestamp DATETIME, " +
		"initiator_id TEXT," +
		"account_id TEXT," +
		"meta TEXT," +
		" target_id TEXT);"

	selectDescQuery = "SELECT id, activity, timestamp, initiator_id, target_id, account_id, meta" +
		" FROM events WHERE account_id = ? ORDER BY timestamp DESC LIMIT ? OFFSET ?;"
	selectAscQuery = "SELECT id, activity, timestamp, initiator_id, target_id, account_id, meta" +
		" FROM events WHERE account_id = ? ORDER BY timestamp ASC LIMIT ? OFFSET ?;"
	insertQuery = "INSERT INTO events(activity, timestamp, initiator_id, target_id, account_id, meta) " +
		"VALUES(?, ?, ?, ?, ?, ?)"
)

// Store is the implementation of the activity.Store interface backed by SQLite
type Store struct {
	db                  *sql.DB
	insertStatement     *sql.Stmt
	selectAscStatement  *sql.Stmt
	selectDescStatement *sql.Stmt
}

// NewSQLiteStore creates a new Store with an event table if not exists.
func NewSQLiteStore(dataDir string) (*Store, error) {
	dbFile := filepath.Join(dataDir, eventSinkDB)
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(createTableQuery)
	if err != nil {
		return nil, err
	}

	insertStmt, err := db.Prepare(insertQuery)
	if err != nil {
		return nil, err
	}

	selectDescStmt, err := db.Prepare(selectDescQuery)
	if err != nil {
		return nil, err
	}

	selectAscStmt, err := db.Prepare(selectAscQuery)
	if err != nil {
		return nil, err
	}

	return &Store{
		db:                  db,
		insertStatement:     insertStmt,
		selectDescStatement: selectDescStmt,
		selectAscStatement:  selectAscStmt,
	}, nil
}

func processResult(result *sql.Rows) ([]*activity.Event, error) {
	events := make([]*activity.Event, 0)
	for result.Next() {
		var id int64
		var operation activity.Activity
		var timestamp time.Time
		var initiator string
		var target string
		var account string
		var jsonMeta string
		err := result.Scan(&id, &operation, &timestamp, &initiator, &target, &account, &jsonMeta)
		if err != nil {
			return nil, err
		}

		meta := make(map[string]any)
		if jsonMeta != "" {
			err = json.Unmarshal([]byte(jsonMeta), &meta)
			if err != nil {
				return nil, err
			}
		}

		events = append(events, &activity.Event{
			Timestamp:   timestamp,
			Activity:    operation,
			ID:          uint64(id),
			InitiatorID: initiator,
			TargetID:    target,
			AccountID:   account,
			Meta:        meta,
		})
	}

	return events, nil
}

// Get returns "limit" number of events from index ordered descending or ascending by a timestamp
func (store *Store) Get(accountID string, offset, limit int, descending bool) ([]*activity.Event, error) {
	stmt := store.selectDescStatement
	if !descending {
		stmt = store.selectAscStatement
	}

	result, err := stmt.Query(accountID, limit, offset)
	if err != nil {
		return nil, err
	}

	defer result.Close() //nolint
	return processResult(result)
}

// Save an event in the SQLite events table
func (store *Store) Save(event *activity.Event) (*activity.Event, error) {
	var jsonMeta string
	if event.Meta != nil {
		metaBytes, err := json.Marshal(event.Meta)
		if err != nil {
			return nil, err
		}
		jsonMeta = string(metaBytes)
	}

	result, err := store.insertStatement.Exec(event.Activity, event.Timestamp, event.InitiatorID, event.TargetID, event.AccountID, jsonMeta)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	eventCopy := event.Copy()
	eventCopy.ID = uint64(id)
	return eventCopy, nil
}

// Close the Store
func (store *Store) Close() error {
	if store.db != nil {
		return store.db.Close()
	}
	return nil
}
