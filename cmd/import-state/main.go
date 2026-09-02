package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Zakkaus/vestibule/internal/database"
)

func main() {
	log.SetFlags(0)
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("import-state", flag.ContinueOnError)
	stateDirectory := flags.String("state-dir", os.Getenv("STATE_DIRECTORY"), "directory containing the five legacy JSON files")
	databaseType := flags.String("database-type", os.Getenv("VT_DATABASE_TYPE"), "dbutil driver name (default sqlite3-fk-wal)")
	databaseURI := flags.String("database-uri", os.Getenv("VT_DATABASE_URI"), "database URI (default STATE_DIRECTORY/vestibule.db)")
	backupDirectory := flags.String("backup-dir", "", "exact directory for this import backup")
	pending := flags.String("pending", "", "what to do with the previous generation's open challenges: carry or drop (required)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*stateDirectory) == "" {
		return fmt.Errorf("-state-dir or STATE_DIRECTORY is required")
	}
	db, err := database.Open(ctx, database.Config{
		Type: *databaseType, URI: *databaseURI, StateDirectory: *stateDirectory,
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	report, importErr := database.ImportLegacyState(ctx, db, database.ImportOptions{
		StateDirectory: *stateDirectory, BackupDirectory: *backupDirectory,
		Pending: database.PendingDisposition(strings.TrimSpace(*pending)),
	})
	closeErr := db.Close()
	if importErr != nil {
		return fmt.Errorf("import legacy state: %w", importErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close database: %w", closeErr)
	}
	fmt.Printf("backup: %s\n%s\n", report.BackupDirectory, report.ValidationText())
	return nil
}
