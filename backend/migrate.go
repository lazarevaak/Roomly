package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func migrate(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("не удалось начать миграцию: %w", err)
	}
	defer tx.Rollback()

	files, err := migrationFiles("migrations")
	if err != nil {
		return err
	}

	for _, file := range files {
		query, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("не удалось прочитать миграцию %s: %w", file, err)
		}

		if _, err := tx.ExecContext(ctx, string(query)); err != nil {
			return fmt.Errorf("ошибка выполнения миграции %s: %w", file, err)
		}

		log.Printf("Миграция применена: %s", filepath.Base(file))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("не удалось сохранить миграцию: %w", err)
	}

	log.Println("Миграция выполнена: схема приложения готова")
	return nil
}

func migrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать папку миграций %s: %w", dir, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		files = append(files, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("в папке %s нет SQL-миграций", dir)
	}

	return files, nil
}
