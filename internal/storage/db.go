package storage

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/txarli-san/codeassist/internal/scanner/parsers"
)

// DB wraps the database connection
type DB struct {
	conn   *sql.DB
	hasFTS bool // Flag to indicate if FTS is available
}

// InitDB initializes the database
func InitDB(dbFile string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}

	// Create basic tables
	_, err = conn.Exec(`
CREATE TABLE IF NOT EXISTS files (
    id INTEGER PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    language TEXT NOT NULL,
    last_modified INTEGER NOT NULL,
    size INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS entities (
    id INTEGER PRIMARY KEY,
    file_id INTEGER NOT NULL,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    signature TEXT,
    line_start INTEGER NOT NULL,
    line_end INTEGER NOT NULL,
    content TEXT NOT NULL,
    description TEXT,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);
CREATE INDEX IF NOT EXISTS idx_entities_file_id ON entities(file_id);
CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(type);
`)
	if err != nil {
		conn.Close()
		return nil, err
	}

	db := &DB{conn: conn, hasFTS: false}

	// Try to create FTS virtual table - this might fail if FTS is not available
	_, err = conn.Exec(`
CREATE VIRTUAL TABLE IF NOT EXISTS entities_fts USING fts4(
    name, content, description,
    content='entities',
    content_rowid='id'
);
	`)

	if err == nil {
		// FTS is available, create triggers
		_, err = conn.Exec(`
CREATE TRIGGER IF NOT EXISTS entities_ai AFTER INSERT ON entities BEGIN
  INSERT INTO entities_fts(rowid, name, content, description) VALUES (new.id, new.name, new.content, new.description);
END;

CREATE TRIGGER IF NOT EXISTS entities_ad AFTER DELETE ON entities BEGIN
  INSERT INTO entities_fts(entities_fts, rowid, name, content, description) VALUES('delete', old.id, old.name, old.content, old.description);
END;

CREATE TRIGGER IF NOT EXISTS entities_au AFTER UPDATE ON entities BEGIN
  INSERT INTO entities_fts(entities_fts, rowid, name, content, description) VALUES('delete', old.id, old.name, old.content, old.description);
  INSERT INTO entities_fts(rowid, name, content, description) VALUES (new.id, new.name, new.content, new.description);
END;
		`)

		if err == nil {
			db.hasFTS = true
			fmt.Println("Full-text search enabled")
		} else {
			fmt.Println("Failed to create FTS triggers, using basic search")
		}
	} else {
		fmt.Println("Full-text search not available, using basic search")
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// StoreFileAndEntities stores a file and its entities in the database
func (db *DB) StoreFileAndEntities(path, language string, modTime int64, size int, entities []parsers.Entity) error {
	// Begin transaction
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Store file
	var fileID int64
	stmt, err := tx.Prepare("INSERT OR REPLACE INTO files (path, language, last_modified, size) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	res, err := stmt.Exec(path, language, modTime, size)
	if err != nil {
		return err
	}
	fileID, err = res.LastInsertId()
	if err != nil {
		return err
	}

	// Delete existing entities for this file
	_, err = tx.Exec("DELETE FROM entities WHERE file_id = ?", fileID)
	if err != nil {
		return err
	}

	// Store entities
	stmt, err = tx.Prepare("INSERT INTO entities (file_id, type, name, signature, line_start, line_end, content, description) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	for _, entity := range entities {
		_, err = stmt.Exec(fileID, entity.Type, entity.Name, entity.Signature, entity.LineStart, entity.LineEnd, entity.Content, entity.Description)
		if err != nil {
			return err
		}
	}

	// Commit transaction
	return tx.Commit()
}

// FindRelevantEntities finds entities relevant to a query
func (db *DB) FindRelevantEntities(query string) ([]parsers.Entity, error) {
	var rows *sql.Rows
	var err error

	// Use FTS if available
	if db.hasFTS {
		rows, err = db.conn.Query(`
SELECT e.type, e.name, e.signature, e.line_start, e.line_end, e.content, e.description, f.path
FROM entities_fts fts
JOIN entities e ON fts.rowid = e.id
JOIN files f ON e.file_id = f.id
WHERE entities_fts MATCH ?
LIMIT 10
`, query)
	}

	// If FTS fails or is not available, fall back to LIKE
	if !db.hasFTS || err != nil {
		keywords := strings.Fields(strings.ToLower(query))

		var whereClause strings.Builder
		whereClause.WriteString("WHERE ")

		for i, keyword := range keywords {
			if i > 0 {
				whereClause.WriteString(" OR ")
			}
			whereClause.WriteString(fmt.Sprintf("(LOWER(e.name) LIKE '%%%s%%' OR LOWER(e.description) LIKE '%%%s%%' OR LOWER(e.content) LIKE '%%%s%%')", keyword, keyword, keyword))
		}

		// Execute fallback query
		rows, err = db.conn.Query(fmt.Sprintf(`
SELECT e.type, e.name, e.signature, e.line_start, e.line_end, e.content, e.description, f.path
FROM entities e
JOIN files f ON e.file_id = f.id
%s
LIMIT 10`, whereClause.String()))
		if err != nil {
			return nil, err
		}
	}

	defer rows.Close()

	var entities []parsers.Entity
	for rows.Next() {
		var e parsers.Entity
		var path string
		if err := rows.Scan(&e.Type, &e.Name, &e.Signature, &e.LineStart, &e.LineEnd, &e.Content, &e.Description, &path); err != nil {
			return nil, err
		}
		// Add file path to description for context
		if e.Description != "" {
			e.Description += "\n\n"
		}
		e.Description += fmt.Sprintf("File: %s", path)
		entities = append(entities, e)
	}

	return entities, nil
}

// GetStats returns database statistics
func (db *DB) GetStats() (map[string]int, error) {
	stats := make(map[string]int)

	// Count files
	var fileCount int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)
	if err != nil {
		return nil, err
	}
	stats["files"] = fileCount

	// Count entities
	var entityCount int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM entities").Scan(&entityCount)
	if err != nil {
		return nil, err
	}
	stats["entities"] = entityCount

	// Count by entity type
	rows, err := db.conn.Query("SELECT type, COUNT(*) FROM entities GROUP BY type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var entityType string
		var count int
		if err := rows.Scan(&entityType, &count); err != nil {
			return nil, err
		}
		stats["type_"+entityType] = count
	}

	// Count by language
	rows, err = db.conn.Query("SELECT language, COUNT(*) FROM files GROUP BY language")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var language string
		var count int
		if err := rows.Scan(&language, &count); err != nil {
			return nil, err
		}
		stats["lang_"+language] = count
	}

	return stats, nil
}
