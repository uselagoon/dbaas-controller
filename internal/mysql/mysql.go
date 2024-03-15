package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

	_ "github.com/go-sql-driver/mysql"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Ping pings the MySQL database
func Ping(ctx context.Context, dsn string) error {
	log.FromContext(ctx).Info("Pinging MySQL database")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("ping failed to open MySQL database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping MySQL database: %w", err)
	}

	return nil
}

// Version returns the version of the MySQL database
func Version(ctx context.Context, dsn string) (string, error) {
	log.FromContext(ctx).Info("Getting MySQL database version")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return "", fmt.Errorf("version failed to open MySQL database: %w", err)
	}
	defer db.Close()

	var version string
	err = db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("version failed to get MySQL database version: %w", err)
	}

	return version, nil
}

// CreateDatabase creates a database in the MySQL database if it does not exist.
// It also creates a user and grants the user permissions on the database.
// This function is idempotent and can be called multiple times without side effects.
func CreateDatabase(ctx context.Context, dsn, databaseName, userName, password string) error {
	log.FromContext(ctx).Info("Creating MySQL database")
	// Connect to the database server
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("create database error connecting to the database server: %w", err)
	}
	defer db.Close()

	// Ping the database to verify connection establishment.
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("create database error verifying connection to the database server: %w", err)
	}

	// Create the database
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", databaseName))
	if err != nil {
		return fmt.Errorf("create database error in creating the database `%s`: %w", databaseName, err)
	}

	// Create the user and grant permissions
	// Use prepared statements to avoid SQL injection vulnerabilities.
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY ?", userName), password)
	if err != nil {
		return fmt.Errorf("create database error creating user `%s`: %w", userName, err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, LOCK TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `%s`.* TO '%s'@'%%'", databaseName, userName))
	if err != nil {
		return fmt.Errorf(
			"create database error granting privileges to user `%s` on database `%s`: %w", userName, databaseName, err)
	}

	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		return fmt.Errorf("create database error flushing privileges: %w", err)
	}

	return nil
}

// This regex allows letters, numbers, underscores, and $, which are typical for MySQL identifiers.
var re = regexp.MustCompile(`^[0-9a-zA-Z$_]+$`)

// isValidIdentifier checks if the string contains only allowed characters
// and does not exceed MySQL's maximum length for identifiers.
func isValidIdentifier(name string) bool {
	if len(name) > 64 {
		return false
	}
	return re.MatchString(name)
}
