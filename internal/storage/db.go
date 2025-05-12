package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/txarli-san/codeassist/internal/scanner/parsers"
)

type DB struct {
	conn   *sql.DB
	hasFTS bool
}

func InitDB(dbFile string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbFile+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	_, err = conn.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		fmt.Printf("Warning: Failed to enable WAL mode: %v\n", err)
	}

	schemaSQL := `
	CREATE TABLE IF NOT EXISTS files (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    path TEXT NOT NULL UNIQUE,
	    language TEXT NOT NULL,
	    last_modified INTEGER NOT NULL,
	    size INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS entities (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    file_id INTEGER NOT NULL,
	    type TEXT NOT NULL,
	    name TEXT NOT NULL,
	    signature TEXT,
	    line_start INTEGER NOT NULL,
	    line_end INTEGER NOT NULL,
	    content TEXT NOT NULL,
	    description TEXT,
	    package_context TEXT,
	    signature_json TEXT,
	    ast_metadata_json TEXT,
	    receiver_type TEXT,
	    param_count INTEGER,
	    return_count INTEGER,
	    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS whole_files (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    path TEXT NOT NULL UNIQUE,
	    language TEXT NOT NULL,
	    last_modified INTEGER NOT NULL,
	    content TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS file_imports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id INTEGER NOT NULL,
		import_path TEXT NOT NULL,
		alias TEXT,
		FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
		UNIQUE(file_id, import_path)
	);

	CREATE TABLE IF NOT EXISTS entity_relationships (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_entity_id INTEGER NOT NULL,
		target_entity_name TEXT NOT NULL,
		target_entity_context TEXT,
		target_entity_id INTEGER,
		relationship_type TEXT NOT NULL,
		line_number INTEGER,
		details_json TEXT,
		FOREIGN KEY (source_entity_id) REFERENCES entities(id) ON DELETE CASCADE,
		FOREIGN KEY (target_entity_id) REFERENCES entities(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
	CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);
	CREATE INDEX IF NOT EXISTS idx_entities_file_id ON entities(file_id);
	CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(type);
	CREATE INDEX IF NOT EXISTS idx_entities_receiver_type ON entities(receiver_type);
	CREATE INDEX IF NOT EXISTS idx_whole_files_path ON whole_files(path);
	CREATE INDEX IF NOT EXISTS idx_whole_files_language ON whole_files(language);
	CREATE INDEX IF NOT EXISTS idx_file_imports_file_id ON file_imports(file_id);
	CREATE INDEX IF NOT EXISTS idx_entity_relationships_source ON entity_relationships(source_entity_id, relationship_type);
	CREATE INDEX IF NOT EXISTS idx_entity_relationships_target_id ON entity_relationships(target_entity_id, relationship_type);
	CREATE INDEX IF NOT EXISTS idx_entity_relationships_target_name ON entity_relationships(target_entity_name, relationship_type);
	`
	_, err = conn.Exec(schemaSQL)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("error creating base tables: %w", err)
	}

	db := &DB{conn: conn, hasFTS: false}

	var ftsEnabledOption bool
	err = conn.QueryRow("SELECT EXISTS (SELECT 1 FROM pragma_compile_options WHERE compile_options LIKE 'ENABLE_FTS5')").Scan(&ftsEnabledOption)
	if err != nil || !ftsEnabledOption {
		log.Println("Warning: FTS5 module not available or check failed. Full-text search will use basic LIKE queries.")
		db.hasFTS = false
	} else {
		err = db.setupFTS(nil)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("error setting up FTS: %w", err)
		}
		db.hasFTS = true
		fmt.Println("Full-text search (FTS5) enabled for entities.")
	}

	return db, nil
}

func (db *DB) setupFTS(tx *sql.Tx) error {
	var execer interface {
		Exec(query string, args ...interface{}) (sql.Result, error)
	}
	execer = db.conn
	if tx != nil {
		execer = tx
	}

	_, _ = execer.Exec(`DROP TRIGGER IF EXISTS entities_ai;`)
	_, _ = execer.Exec(`DROP TRIGGER IF EXISTS entities_ad;`)
	_, _ = execer.Exec(`DROP TRIGGER IF EXISTS entities_au;`)

	_, err := execer.Exec(`DROP TABLE IF EXISTS entities_fts;`)
	if err != nil {
		return fmt.Errorf("failed to drop existing entities_fts table: %w", err)
	}

	_, err = execer.Exec(`
	CREATE VIRTUAL TABLE entities_fts USING fts5(
		name,
		content,
		description,
		package_context,
		receiver_type,
		content='entities',
		content_rowid='id',
		tokenize = "porter unicode61"
	);`)
	if err != nil {
		return fmt.Errorf("failed to create entities_fts table: %w", err)
	}

	_, err = execer.Exec(`
	CREATE TRIGGER entities_ai AFTER INSERT ON entities BEGIN
	  INSERT INTO entities_fts(rowid, name, content, description, package_context, receiver_type) VALUES (
		new.id,
		new.name,
		new.content,
		new.description,
		new.package_context,
		new.receiver_type
	  );
	END;`)
	if err != nil {
		return fmt.Errorf("failed to create entities_ai trigger: %w", err)
	}

	_, err = execer.Exec(`
	CREATE TRIGGER entities_ad AFTER DELETE ON entities BEGIN
	  INSERT INTO entities_fts(entities_fts, rowid, name, content, description, package_context, receiver_type) VALUES(
		'delete',
		old.id,
		old.name,
		old.content,
		old.description,
		old.package_context,
		old.receiver_type
	  );
	END;`)
	if err != nil {
		return fmt.Errorf("failed to create entities_ad trigger: %w", err)
	}

	_, err = execer.Exec(`
	CREATE TRIGGER entities_au AFTER UPDATE ON entities BEGIN
	  UPDATE entities_fts SET
	    name = new.name,
	    content = new.content,
	    description = new.description,
	    package_context = new.package_context,
	    receiver_type = new.receiver_type
	  WHERE rowid = old.id;
	END;`)
	if err != nil {
		return fmt.Errorf("failed to create entities_au trigger: %w", err)
	}

	if tx == nil {
		_, err = db.conn.Exec(`INSERT INTO entities_fts (rowid, name, content, description, package_context, receiver_type)
			SELECT id, name, content, description, package_context, receiver_type FROM entities;`)
		if err != nil {

			queryErr := db.conn.QueryRow("SELECT COUNT(*) FROM entities").Scan(&[]int{}[0])
			if queryErr != sql.ErrNoRows && queryErr != nil && !strings.Contains(queryErr.Error(), "no such table") && !strings.Contains(queryErr.Error(), "malformed database schema") {
				log.Printf("Warning: Initial population of FTS table failed: %v", err)
			}
		}
	}
	return nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) StoreFileAndEntities(path, language string, modTime int64, size int, richEntities []parsers.RichEntity) (err error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed && err != nil {
			rbErr := tx.Rollback()
			if rbErr != nil {
				log.Printf("Error rolling back transaction for path %s: %v (original error: %v)", path, rbErr, err)
			}
		} else if !committed && err == nil {

			rbErr := tx.Rollback()
			if rbErr != nil {
				log.Printf("Error rolling back transaction for path %s due to no commit: %v", path, rbErr)
			}
		}
	}()

	var fileID int64
	err = tx.QueryRow("SELECT id FROM files WHERE path = ?", path).Scan(&fileID)
	if err == sql.ErrNoRows {
		res, errExec := tx.Exec("INSERT INTO files (path, language, last_modified, size) VALUES (?, ?, ?, ?)",
			path, language, modTime, size)
		if errExec != nil {
			err = fmt.Errorf("insert file '%s': %w", path, errExec)
			return err
		}
		fileID, err = res.LastInsertId()
		if err != nil {

			errQuery := tx.QueryRow("SELECT id FROM files WHERE path = ?", path).Scan(&fileID)
			if errQuery != nil {
				err = fmt.Errorf("get last insert id or query existing for file '%s': %w (original LastInsertId error: %v)", path, errQuery, err)
				return err
			}
		}
	} else if err != nil {
		err = fmt.Errorf("query file id for path '%s': %w", path, err)
		return err
	} else {
		_, err = tx.Exec("UPDATE files SET last_modified = ?, size = ? WHERE id = ?", modTime, size, fileID)
		if err != nil {
			err = fmt.Errorf("update file id %d: %w", fileID, err)
			return err
		}
	}

	_, err = tx.Exec("DELETE FROM entities WHERE file_id = ?", fileID)
	if err != nil {
		err = fmt.Errorf("delete old entities for file_id %d: %w", fileID, err)
		return err
	}
	_, err = tx.Exec("DELETE FROM file_imports WHERE file_id = ?", fileID)
	if err != nil {
		err = fmt.Errorf("delete old file imports for file_id %d: %w", fileID, err)
		return err
	}

	importStmt, err := tx.Prepare("INSERT OR IGNORE INTO file_imports (file_id, import_path, alias) VALUES (?, ?, ?)")
	if err != nil {
		err = fmt.Errorf("prepare file_import insert: %w", err)
		return err
	}
	defer importStmt.Close()

	processedFileImports := make(map[string]bool)
	if len(richEntities) > 0 && len(richEntities[0].Imports) > 0 {
		for _, importInfo := range richEntities[0].Imports {
			importKey := importInfo.Path + "###" + importInfo.Alias
			if !processedFileImports[importKey] {
				_, errExec := importStmt.Exec(fileID, importInfo.Path, importInfo.Alias)
				if errExec != nil {
					err = fmt.Errorf("insert file_import for file_id %d (%s): %w", fileID, importInfo.Path, errExec)
					return err
				}
				processedFileImports[importKey] = true
			}
		}
	}

	entityStmt, err := tx.Prepare(`
		INSERT INTO entities (
			file_id, type, name, signature, line_start, line_end, content, description,
			package_context, signature_json, ast_metadata_json, receiver_type, param_count, return_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		err = fmt.Errorf("prepare entity insert: %w", err)
		return err
	}
	defer entityStmt.Close()

	relStmt, err := tx.Prepare(`
		INSERT INTO entity_relationships (
			source_entity_id, target_entity_name, target_entity_context, target_entity_id,
			relationship_type, line_number, details_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		err = fmt.Errorf("prepare relationship insert: %w", err)
		return err
	}
	defer relStmt.Close()

	for i, re := range richEntities {
		if re.Entity.Name == "" && re.Entity.Type != "html" && re.Entity.Type != "template" && re.Entity.Type != "package" {
			continue
		}

		signatureForDB := sql.NullString{String: re.Entity.Signature, Valid: re.Entity.Signature != ""}
		descriptionForDB := sql.NullString{String: re.Entity.Description, Valid: re.Entity.Description != ""}
		packageContextForDB := sql.NullString{String: re.PackageName, Valid: re.PackageName != ""}
		signatureJSONForDB := sql.NullString{String: re.SignatureJSON, Valid: re.SignatureJSON != "" && re.SignatureJSON != "{}" && re.SignatureJSON != "null"}
		astMetadataJSONForDB := sql.NullString{String: re.ASTMetadata, Valid: re.ASTMetadata != "" && re.ASTMetadata != "{}" && re.ASTMetadata != "null"}
		receiverTypeForDB := sql.NullString{String: re.ReceiverType, Valid: re.ReceiverType != ""}
		paramCountForDB := sql.NullInt64{Int64: int64(re.ParamCount), Valid: true}
		returnCountForDB := sql.NullInt64{Int64: int64(re.ReturnCount), Valid: true}

		if re.Entity.LineStart == 0 {
			re.Entity.LineStart = 1
		}
		if re.Entity.LineEnd == 0 {
			re.Entity.LineEnd = 1
		}
		if re.Entity.LineEnd < re.Entity.LineStart {
			re.Entity.LineEnd = re.Entity.LineStart
		}

		entityRes, errExec := entityStmt.Exec(
			fileID, re.Entity.Type, re.Entity.Name, signatureForDB,
			re.Entity.LineStart, re.Entity.LineEnd, re.Entity.Content, descriptionForDB,
			packageContextForDB, signatureJSONForDB, astMetadataJSONForDB, receiverTypeForDB,
			paramCountForDB, returnCountForDB,
		)
		if errExec != nil {
			err = fmt.Errorf("insert entity '%s' #%d for file_id %d: %w", re.Entity.Name, i, fileID, errExec)
			return err
		}

		entityID, errLII := entityRes.LastInsertId()
		if errLII != nil || entityID == 0 {
			errQuery := tx.QueryRow("SELECT id FROM entities WHERE file_id = ? AND type = ? AND name = ? AND line_start = ? ORDER BY id DESC LIMIT 1",
				fileID, re.Entity.Type, re.Entity.Name, re.Entity.LineStart).Scan(&entityID)
			if errQuery != nil {
				err = fmt.Errorf("error getting entity ID for '%s' (file %d, type %s, line %d): %w (original LastInsertId error: %v)", re.Entity.Name, fileID, re.Entity.Type, re.Entity.LineStart, errQuery, errLII)
				return err
			}
			if entityID == 0 {
				err = fmt.Errorf("failed to obtain valid entity ID for '%s' (file %d, type %s, line %d)", re.Entity.Name, fileID, re.Entity.Type, re.Entity.LineStart)
				return err
			}
		}

		for _, call := range re.Calls {
			var detailsJSON sql.NullString
			if len(call.Arguments) > 0 {
				detailsBytes, marshalErr := json.Marshal(call.Arguments)
				if marshalErr == nil {
					detailsJSON.String = string(detailsBytes)
					detailsJSON.Valid = true
				}
			}

			var targetEntityID sql.NullInt64
			if call.TargetResolvedID != nil {
				targetEntityID.Int64 = *call.TargetResolvedID
				targetEntityID.Valid = true
			}

			_, errExecRel := relStmt.Exec(
				entityID, call.TargetName, call.TargetContext, targetEntityID,
				"CALLS", call.LineNumber, detailsJSON,
			)
			if errExecRel != nil {
				err = fmt.Errorf("insert call relationship for entity %s (id %d): %w", re.Entity.Name, entityID, errExecRel)
				return err
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return nil
}

func (db *DB) FindRelevantEntities(query string) ([]parsers.Entity, error) {
	var rows *sql.Rows
	var err error
	limit := 15

	finalQueryArgs := []interface{}{}

	baseSelect := `
		SELECT e.type, e.name, e.signature, e.line_start, e.line_end, e.content, e.description, f.path,
			   e.package_context, e.receiver_type, e.signature_json, e.ast_metadata_json, e.param_count, e.return_count
		FROM entities e
		JOIN files f ON e.file_id = f.id`

	var ftsMatchClause string
	var orderByClause string

	if db.hasFTS {
		ftsMatchClause = ` JOIN entities_fts fts ON fts.rowid = e.id WHERE entities_fts MATCH ? `
		orderByClause = ` ORDER BY rank LIMIT ? `
		finalQueryArgs = append(finalQueryArgs, query, limit)
	} else {
		keywords := strings.Fields(strings.ToLower(query))
		if len(keywords) == 0 {
			return []parsers.Entity{}, nil
		}
		var whereConditions []string
		for _, keyword := range keywords {
			likeKeyword := "%" + keyword + "%"
			whereConditions = append(whereConditions, "(LOWER(e.name) LIKE ? OR LOWER(e.description) LIKE ? OR LOWER(e.content) LIKE ? OR LOWER(e.package_context) LIKE ? OR LOWER(e.receiver_type) LIKE ?)")
			finalQueryArgs = append(finalQueryArgs, likeKeyword, likeKeyword, likeKeyword, likeKeyword, likeKeyword)
		}
		ftsMatchClause = fmt.Sprintf(" WHERE %s ", strings.Join(whereConditions, " OR "))
		orderByClause = ` ORDER BY e.id DESC LIMIT ? `
		finalQueryArgs = append(finalQueryArgs, limit)
	}

	fullQueryString := baseSelect + ftsMatchClause + orderByClause
	rows, err = db.conn.Query(fullQueryString, finalQueryArgs...)
	if err != nil {
		if db.hasFTS {
			fmt.Printf("FTS query failed ('%s'), falling back to LIKE: %v\n", query, err)

			keywords := strings.Fields(strings.ToLower(query))
			if len(keywords) == 0 {
				return []parsers.Entity{}, nil
			}
			var whereConditions []string
			var args []interface{}
			for _, keyword := range keywords {
				likeKeyword := "%" + keyword + "%"
				whereConditions = append(whereConditions, "(LOWER(e.name) LIKE ? OR LOWER(e.description) LIKE ? OR LOWER(e.content) LIKE ? OR LOWER(e.package_context) LIKE ? OR LOWER(e.receiver_type) LIKE ?)")
				args = append(args, likeKeyword, likeKeyword, likeKeyword, likeKeyword, likeKeyword)
			}
			args = append(args, limit)

			fallbackQueryString := fmt.Sprintf(`
				SELECT e.type, e.name, e.signature, e.line_start, e.line_end, e.content, e.description, f.path,
					   e.package_context, e.receiver_type, e.signature_json, e.ast_metadata_json, e.param_count, e.return_count
				FROM entities e
				JOIN files f ON e.file_id = f.id
				WHERE %s
				ORDER BY e.id DESC
				LIMIT ?`, strings.Join(whereConditions, " OR "))

			rows, err = db.conn.Query(fallbackQueryString, args...)
			if err != nil {
				return nil, fmt.Errorf("fallback LIKE query failed: %w", err)
			}
		} else {
			return nil, fmt.Errorf("query failed: %w", err)
		}
	}
	defer rows.Close()

	var resultEntities []parsers.Entity
	for rows.Next() {
		var pe parsers.Entity

		var path, packageContext, receiverType, signatureJSON, astMetadataJSON sql.NullString
		var signature, description sql.NullString
		var paramCount, returnCount sql.NullInt64

		scanArgs := []interface{}{
			&pe.Type, &pe.Name, &signature, &pe.LineStart, &pe.LineEnd,
			&pe.Content, &description, &path, &packageContext, &receiverType,
			&signatureJSON, &astMetadataJSON, &paramCount, &returnCount,
		}

		if db.hasFTS && strings.Contains(strings.ToLower(fullQueryString), "fts.rank") {
		}

		if errScan := rows.Scan(scanArgs...); errScan != nil {
			return nil, fmt.Errorf("scanning entity row: %w", errScan)
		}

		pe.Signature = signature.String
		pe.Description = description.String

		descBuilder := strings.Builder{}
		if pe.Description != "" {
			descBuilder.WriteString(pe.Description)
		}

		filePath := path.String
		if filePath != "" {
			if descBuilder.Len() > 0 {
				descBuilder.WriteString("\n")
			}
			descBuilder.WriteString(fmt.Sprintf("File: %s", filePath))
		}
		if packageContext.Valid && packageContext.String != "" {
			if descBuilder.Len() > 0 {
				descBuilder.WriteString("\n")
			}
			descBuilder.WriteString(fmt.Sprintf("Package: %s", packageContext.String))
		}
		if receiverType.Valid && receiverType.String != "" {
			if descBuilder.Len() > 0 {
				descBuilder.WriteString("\n")
			}
			descBuilder.WriteString(fmt.Sprintf("Receiver: %s", receiverType.String))
		}

		pe.Description = descBuilder.String()
		resultEntities = append(resultEntities, pe)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return resultEntities, nil
}

func (db *DB) GetStats() (map[string]int, error) {
	stats := make(map[string]int)

	countQuery := func(tableName string) (int, error) {
		var count int
		err := db.conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count)
		return count, err
	}

	var err error
	stats["files"], err = countQuery("files")
	if err != nil {
		return nil, fmt.Errorf("count files: %w", err)
	}

	stats["entities"], err = countQuery("entities")
	if err != nil {
		return nil, fmt.Errorf("count entities: %w", err)
	}

	stats["whole_files"], err = countQuery("whole_files")
	if err != nil {
		return nil, fmt.Errorf("count whole_files: %w", err)
	}

	stats["file_imports"], err = countQuery("file_imports")
	if err != nil {
		return nil, fmt.Errorf("count file_imports: %w", err)
	}

	stats["entity_relationships"], err = countQuery("entity_relationships")
	if err != nil {
		return nil, fmt.Errorf("count entity_relationships: %w", err)
	}

	groupQuery := func(table, groupColumn, prefix string) error {
		rows, errGroup := db.conn.Query(fmt.Sprintf("SELECT %s, COUNT(*) FROM %s GROUP BY %s", groupColumn, table, groupColumn))
		if errGroup != nil {
			return errGroup
		}
		defer rows.Close()
		for rows.Next() {
			var value string
			var count int
			if errScan := rows.Scan(&value, &count); errScan != nil {
				return errScan
			}
			stats[prefix+value] = count
		}
		return rows.Err()
	}

	if err = groupQuery("entities", "type", "type_"); err != nil {
		return nil, fmt.Errorf("count entities by type: %w", err)
	}

	if err = groupQuery("files", "language", "lang_"); err != nil {
		return nil, fmt.Errorf("count files by language: %w", err)
	}

	if err = groupQuery("entity_relationships", "relationship_type", "relationship_"); err != nil {
		return nil, fmt.Errorf("count relationships by type: %w", err)
	}

	return stats, nil
}

func (db *DB) StoreWholeFile(path, language string, modTime int64, content string) error {
	_, err := db.conn.Exec(
		"INSERT OR REPLACE INTO whole_files (path, language, last_modified, content) VALUES (?, ?, ?, ?)",
		path, language, modTime, content,
	)
	if err != nil {
		return fmt.Errorf("store whole file %s: %w", path, err)
	}
	return nil
}

func (db *DB) FindRelevantFiles(query string, limit int) ([]map[string]interface{}, error) {
	keywords := strings.Fields(strings.ToLower(query))
	if len(keywords) == 0 {
		return []map[string]interface{}{}, nil
	}

	var sb strings.Builder
	sb.WriteString(`
        WITH relevance AS (
            SELECT
                wf.path,
                wf.language,
                wf.content,
                (`)

	var queryParams []interface{}
	for i, keyword := range keywords {
		if i > 0 {
			sb.WriteString(" + ")
		}
		likeKeyword := "%" + keyword + "%"

		sb.WriteString(` CASE WHEN LOWER(wf.path) LIKE ? THEN 3.0 ELSE 0.0 END `)
		queryParams = append(queryParams, likeKeyword)

		sb.WriteString(` + (SELECT 5.0 * COUNT(*) FROM entities e JOIN files f ON e.file_id = f.id WHERE f.path = wf.path AND (LOWER(e.name) LIKE ? OR LOWER(e.description) LIKE ? ))`)
		queryParams = append(queryParams, likeKeyword, likeKeyword)

		escapedKeywordForLen := keyword
		if len(escapedKeywordForLen) == 0 {
			escapedKeywordForLen = " "
		}
		sb.WriteString(` + (CAST(LENGTH(LOWER(wf.content)) - LENGTH(REPLACE(LOWER(wf.content), ?, '')) AS REAL) / LENGTH(?))`)
		queryParams = append(queryParams, keyword, escapedKeywordForLen)

	}
	queryParams = append(queryParams, limit)

	sb.WriteString(`
            ) AS score
        FROM whole_files wf
        WHERE score > 0
        ORDER BY score DESC
        LIMIT ?
    )
    SELECT path, language, content FROM relevance WHERE path IS NOT NULL AND language IS NOT NULL AND content IS NOT NULL
    `)

	rows, err := db.conn.Query(sb.String(), queryParams...)
	if err != nil {
		return nil, fmt.Errorf("querying relevant files: %w", err)
	}
	defer rows.Close()

	var files []map[string]interface{}
	for rows.Next() {
		var path, language, content string
		if err := rows.Scan(&path, &language, &content); err != nil {
			return nil, fmt.Errorf("scanning relevant file row: %w", err)
		}
		files = append(files, map[string]interface{}{
			"path":     path,
			"language": language,
			"content":  content,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error for relevant files: %w", err)
	}
	return files, nil
}

func (db *DB) FindRelatedEntities(filePath string) ([]parsers.Entity, error) {
	var fileID int64
	err := db.conn.QueryRow("SELECT id FROM files WHERE path = ?", filePath).Scan(&fileID)
	if err != nil {
		if err == sql.ErrNoRows {
			return []parsers.Entity{}, nil
		}
		return nil, fmt.Errorf("find file id for related entities '%s': %w", filePath, err)
	}

	rows, err := db.conn.Query(`
        SELECT e.type, e.name, e.signature, e.line_start, e.line_end, e.content, e.description
        FROM entities e
        WHERE e.file_id = ?
        ORDER BY e.line_start
        LIMIT 5`, fileID)
	if err != nil {
		return nil, fmt.Errorf("query related entities for file_id %d: %w", fileID, err)
	}
	defer rows.Close()

	var entities []parsers.Entity
	for rows.Next() {
		var e parsers.Entity
		var signature, description sql.NullString
		if err := rows.Scan(&e.Type, &e.Name, &signature, &e.LineStart, &e.LineEnd, &e.Content, &description); err != nil {
			return nil, fmt.Errorf("scanning related entity row: %w", err)
		}
		e.Signature = signature.String
		e.Description = description.String
		entities = append(entities, e)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error for related entities: %w", err)
	}
	return entities, nil
}

type ChunkedCodeSection struct {
	Path      string
	Language  string
	Content   string
	StartLine int
	EndLine   int
	Score     float64
}

func (db *DB) FindRelevantCodeSections(query string, limit int) ([]ChunkedCodeSection, error) {
	keywords := strings.Fields(strings.ToLower(query))
	if len(keywords) == 0 {
		return []ChunkedCodeSection{}, nil
	}

	var queryBuilder strings.Builder
	var queryArgs []interface{}

	actualLimit := limit
	if actualLimit <= 0 {
		actualLimit = 5
	}
	queryLimit := actualLimit * 5

	selectClause := `
		SELECT
			e.id, e.name, e.type, e.line_start, e.line_end,
			f.path, f.language,
			(SELECT wf.content FROM whole_files wf WHERE wf.path = f.path LIMIT 1) as file_content`

	fromClause := `
		FROM entities e
		JOIN files f ON e.file_id = f.id`

	var whereSubClause string
	var orderByClause string

	if db.hasFTS {
		queryBuilder.WriteString(selectClause)
		queryBuilder.WriteString(", fts.rank AS score")
		queryBuilder.WriteString(`
			FROM entities_fts fts
			JOIN entities e ON fts.rowid = e.id
			JOIN files f ON e.file_id = f.id
			WHERE entities_fts MATCH ? `)
		orderByClause = "ORDER BY score"
		queryArgs = append(queryArgs, query)
	} else {
		queryBuilder.WriteString(selectClause)
		queryBuilder.WriteString(", 10.0 AS score")
		queryBuilder.WriteString(fromClause)

		var conditions []string
		for _, keyword := range keywords {
			likeKeyword := "%" + keyword + "%"
			conditions = append(conditions, "(LOWER(e.name) LIKE ? OR LOWER(e.content) LIKE ? OR LOWER(e.description) LIKE ? OR LOWER(e.package_context) LIKE ? OR LOWER(e.receiver_type) LIKE ?)")
			queryArgs = append(queryArgs, likeKeyword, likeKeyword, likeKeyword, likeKeyword, likeKeyword)
		}
		if len(conditions) == 0 {
			return []ChunkedCodeSection{}, nil
		}
		whereSubClause = "WHERE (" + strings.Join(conditions, " OR ") + ")"
		orderByClause = "ORDER BY e.name, e.line_start"
	}

	queryBuilder.WriteString(" ")
	queryBuilder.WriteString(whereSubClause)
	queryBuilder.WriteString(" ")
	queryBuilder.WriteString(orderByClause)
	queryBuilder.WriteString(" LIMIT ?")
	queryArgs = append(queryArgs, queryLimit)

	rows, err := db.conn.Query(queryBuilder.String(), queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying relevant entities for sections build query '%s': %w", queryBuilder.String(), err)
	}
	defer rows.Close()

	var sections []ChunkedCodeSection

	processedFileAndEntityStartLine := make(map[string]bool)

	for rows.Next() {
		var id int
		var name, entityType string
		var lineStart, lineEnd int
		var path, language string
		var fileContentRaw sql.NullString
		var score float64

		err := rows.Scan(&id, &name, &entityType, &lineStart, &lineEnd, &path, &language, &fileContentRaw, &score)
		if err != nil {
			log.Printf("Warning: scanning section row failed: %v", err)
			continue
		}

		if !fileContentRaw.Valid || fileContentRaw.String == "" {
			continue
		}
		fileContent := fileContentRaw.String

		entityMapKey := fmt.Sprintf("%s-%d", path, lineStart)
		if processedFileAndEntityStartLine[entityMapKey] {
			continue
		}

		lines := strings.Split(fileContent, "\n")
		contextLines := 5

		dbLineStart := lineStart
		dbLineEnd := lineEnd

		if dbLineStart <= 0 {
			dbLineStart = 1
		}
		if dbLineEnd <= 0 {
			dbLineEnd = dbLineStart
		}
		if dbLineEnd < dbLineStart {
			dbLineEnd = dbLineStart
		}

		effectiveStart := max(0, dbLineStart-1-contextLines)
		effectiveEnd := min(len(lines), dbLineEnd+contextLines)

		if effectiveStart >= len(lines) {
			effectiveStart = len(lines) - 1
		}
		if effectiveEnd > len(lines) {
			effectiveEnd = len(lines)
		}
		if effectiveStart < 0 {
			effectiveStart = 0
		}

		if effectiveStart >= effectiveEnd {
			if dbLineStart-1 < len(lines) && dbLineStart-1 >= 0 {
				effectiveStart = dbLineStart - 1
			} else {
				effectiveStart = 0
			}
			if dbLineEnd <= len(lines) && dbLineEnd >= effectiveStart {
				effectiveEnd = dbLineEnd
			} else {
				effectiveEnd = effectiveStart
			}
		}

		var sectionLines []string
		if effectiveStart < effectiveEnd {
			sectionLines = lines[effectiveStart:effectiveEnd]
		} else if effectiveStart < len(lines) {
			sectionLines = []string{lines[effectiveStart]}
			effectiveEnd = effectiveStart + 1
		}

		sectionContent := strings.Join(sectionLines, "\n")

		if !db.hasFTS {
			calculatedScore := 10.0
			for _, keyword := range keywords {
				if strings.Contains(strings.ToLower(name), keyword) {
					calculatedScore += 50.0
				}
				if strings.Contains(strings.ToLower(entityType), keyword) {
					calculatedScore += 20.0
				}
				calculatedScore += float64(strings.Count(strings.ToLower(sectionContent), keyword)) * 1.0
			}
			score = calculatedScore
		}

		sections = append(sections, ChunkedCodeSection{
			Path:      path,
			Language:  language,
			Content:   sectionContent,
			StartLine: effectiveStart + 1,
			EndLine:   effectiveEnd,
			Score:     score,
		})
		processedFileAndEntityStartLine[entityMapKey] = true
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error for sections: %w", err)
	}

	sort.Slice(sections, func(i, j int) bool {
		if sections[i].Score != sections[j].Score {
			return sections[i].Score > sections[j].Score
		}
		if sections[i].Path != sections[j].Path {
			return sections[i].Path < sections[j].Path
		}
		return sections[i].StartLine < sections[j].StartLine
	})

	finalSections := []ChunkedCodeSection{}
	uniquePaths := make(map[string]bool)
	for _, s := range sections {
		if len(finalSections) >= actualLimit {
			if !uniquePaths[s.Path] {
			} else {
				continue
			}
		}
		if !uniquePaths[s.Path] {
			finalSections = append(finalSections, s)
			uniquePaths[s.Path] = true
		} else {
			for idx, existingSection := range finalSections {
				if existingSection.Path == s.Path {
					if s.Score > existingSection.Score {
						finalSections[idx] = s
					}
					break
				}
			}
		}
		if len(finalSections) >= actualLimit && len(uniquePaths) >= actualLimit {
			break
		}
	}

	if len(finalSections) > actualLimit {
		finalSections = finalSections[:actualLimit]
	}

	return finalSections, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
