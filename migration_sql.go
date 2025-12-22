package goose

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Run a migration specified in raw SQL.
//
// Sections of the script can be annotated with a special comment,
// starting with "-- +goose" to specify whether the section should
// be applied during an Up or Down migration
//
// All statements following an Up or Down annotation are grouped together
// until another direction annotation is found.
func runSQLMigration(
	ctx context.Context,
	db *sql.DB,
	statements []string,
	useTx bool,
	v int64,
	direction bool,
	noVersioning bool,
) error {
	if useTx {
		// TRANSACTION.

		verboseInfo("Begin transaction")

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		for _, query := range statements {
			verboseInfo("Executing statement: %s\n", clearStatement(query))
			if _, err := tx.ExecContext(ctx, query); err != nil {
				verboseInfo("Rollback transaction")
				_ = tx.Rollback()
				return fmt.Errorf("failed to execute SQL query %q: %w", clearStatement(query), err)
			}
			if GetDialect() == DialectStarrocks {
				if err := pollStarRocksAlterTable(ctx, tx, query); err != nil {
					verboseInfo("Rollback transaction")
					_ = tx.Rollback()
					return fmt.Errorf("failed to poll starrocks alter table: %w", err)
				}
			}
		}

		if !noVersioning {
			if direction {
				if err := store.InsertVersion(ctx, tx, TableName(), v); err != nil {
					verboseInfo("Rollback transaction")
					_ = tx.Rollback()
					return fmt.Errorf("failed to insert new goose version: %w", err)
				}
			} else {
				if err := store.DeleteVersion(ctx, tx, TableName(), v); err != nil {
					verboseInfo("Rollback transaction")
					_ = tx.Rollback()
					return fmt.Errorf("failed to delete goose version: %w", err)
				}
			}
		}

		verboseInfo("Commit transaction")
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		return nil
	}

	// NO TRANSACTION.
	for _, query := range statements {
		verboseInfo("Executing statement: %s", clearStatement(query))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to execute SQL query %q: %w", clearStatement(query), err)
		}
		if GetDialect() == DialectStarrocks {
			if err := pollStarRocksAlterTable(ctx, db, query); err != nil {
				return fmt.Errorf("failed to poll starrocks alter table: %w", err)
			}
		}
	}
	if !noVersioning {
		if direction {
			if err := store.InsertVersionNoTx(ctx, db, TableName(), v); err != nil {
				return fmt.Errorf("failed to insert new goose version: %w", err)
			}
		} else {
			if err := store.DeleteVersionNoTx(ctx, db, TableName(), v); err != nil {
				return fmt.Errorf("failed to delete goose version: %w", err)
			}
		}
	}

	return nil
}

const (
	grayColor  = "\033[90m"
	resetColor = "\033[00m"
)

func verboseInfo(s string, args ...interface{}) {
	if verbose {
		if noColor {
			log.Printf(s, args...)
		} else {
			log.Printf(grayColor+s+resetColor, args...)
		}
	}
}

var (
	matchSQLComments = regexp.MustCompile(`(?m)^--.*$[\r\n]*`)
	matchEmptyEOL    = regexp.MustCompile(`(?m)^$[\r\n]*`) // TODO: Duplicate
)

func clearStatement(s string) string {
	s = matchSQLComments.ReplaceAllString(s, ``)
	return matchEmptyEOL.ReplaceAllString(s, ``)
}

func pollStarRocksAlterTable(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, _ string) error {

	timeout := time.After(10 * time.Minute)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for StarRocks ALTER to finish")
		case <-ticker.C:
			done, err := isStarRocksAlterComplete(ctx, db)
			if err != nil {
				verboseInfo("error checking alter status: %v", err)
				continue
			}
			if done {
				return nil
			}
		}
	}
}

func isStarRocksAlterComplete(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (bool, error) {
	alterTypes := []string{"COLUMN", "ROLLUP"}

	for _, alterType := range alterTypes {
		q := fmt.Sprintf("SHOW ALTER TABLE %s", alterType)
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			continue // Some StarRocks versions may not support all alter types
		}

		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return false, err
		}

		stateIdx := -1
		for i, col := range cols {
			if strings.EqualFold(col, "State") {
				stateIdx = i
				break
			}
		}

		if stateIdx == -1 {
			rows.Close()
			continue
		}

		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				rows.Close()
				return false, err
			}

			stateStr := ""
			if stateIdx >= 0 {
				switch v := values[stateIdx].(type) {
				case []byte:
					stateStr = string(v)
				case string:
					stateStr = v
				}
			}

			if stateStr != "" && stateStr != "FINISHED" && stateStr != "CANCELLED" {
				verboseInfo("StarRocks %s alter still in progress: %s", alterType, stateStr)
				rows.Close()
				return false, nil
			}
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return false, fmt.Errorf("error iterating alter status rows: %w", err)
		}
		rows.Close()
	}

	return true, nil
}
