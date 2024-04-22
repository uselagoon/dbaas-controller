package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"

	_ "github.com/go-sql-driver/mysql"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DatabaseInfo contains the username, password, and database name
type DatabaseInfo struct {
	// Username is the username for the database
	Username string
	// Password is the password for the database
	Password string
	// Dbname is the database name
	Dbname string
}

// MySQLInterface is the interface for the MySQL database
type MySQLInterface interface {
	// Ping pings the MySQL database
	Ping(ctx context.Context, dsn string) error

	// Version returns the version of the MySQL database
	Version(ctx context.Context, dsn string) (string, error)

	// Load of the MySQL database measured in MB of data and index size.
	// Higher values indicate more data and indexes.
	Load(ctx context.Context, dsn string) (int, error)

	// Initialize initializes the MySQL database
	// This is used by the database MySQL provider to initialize the MySQL database.
	// It does setup the dbass_controller database.
	// This function is idempotent and can be called multiple times without side effects.
	Initialize(ctx context.Context, dsn string) error

	// CreateDatabase creates a database in the MySQL database if it does not exist.
	// It also creates a user and grants the user permissions on the database.
	// This function is idempotent and can be called multiple times without side effects.
	// returns the database name, username, and password
	CreateDatabase(ctx context.Context, dsn, name, namespace string) (DatabaseInfo, error)

	// DropDatabase drops a database in the MySQL database if it exists.
	// This function is idempotent and can be called multiple times without side effects.
	DropDatabase(ctx context.Context, dsn, name, namespace string) error

	// GetDatabase returns the database name, username, and password for the given name and namespace.
	GetDatabase(ctx context.Context, dsn, name, namespace string) (DatabaseInfo, error)
}

// MySQLImpl is the implementation of the MySQL database
type MySQLImpl struct{}

// Make sure MySQLImpl implements MySQLInterface
var _ MySQLInterface = (*MySQLImpl)(nil)

// Ping pings the MySQL database
func (mi *MySQLImpl) Ping(ctx context.Context, dsn string) error {
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
func (mi *MySQLImpl) Version(ctx context.Context, dsn string) (string, error) {
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

// Load returns the load of the MySQL database measured in MB of data and index size.
// Note it doesn't include CPU or memory usage which could be obtained from other sources.
func (mi *MySQLImpl) Load(ctx context.Context, dsn string) (int, error) {
	log.FromContext(ctx).Info("Getting MySQL database load")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return 0, fmt.Errorf("load failed to open MySQL database: %w", err)
	}
	defer db.Close()

	query := `
        SELECT SUM(data_length + index_length) AS total_size
        FROM information_schema.TABLES
    `
	var totalLoad float64
	row := db.QueryRow(query)
	err = row.Scan(&totalLoad)
	if err != nil {
		return 0, fmt.Errorf("load failed to get MySQL database load: %w", err)
	}
	totalLoadMB := totalLoad / (1024 * 1024)
	return int(totalLoadMB), nil
}

// Initialize sets up the dbaas_controller database and the users table.
func (mi *MySQLImpl) Initialize(ctx context.Context, dsn string) error {
	log.FromContext(ctx).Info("Initializing MySQL database")

	// Connect to MySQL server without specifying a database
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create the database if it doesn't exist
	_, err = db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS dbaas_controller")
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// Select the database
	_, err = db.ExecContext(ctx, "USE dbaas_controller")
	if err != nil {
		return fmt.Errorf("failed to select database: %w", err)
	}

	// Create the users table if it doesn't exist
	createTableSQL := `
    CREATE TABLE IF NOT EXISTS users (
        id INT AUTO_INCREMENT PRIMARY KEY,
        name VARCHAR(255) NOT NULL,
        namespace VARCHAR(255) NOT NULL,
        username VARCHAR(16) NOT NULL UNIQUE,
		password VARCHAR(255) NOT NULL,
        dbname VARCHAR(255) NOT NULL UNIQUE,
        CONSTRAINT unique_name_namespace UNIQUE (name, namespace)
    ) ENGINE=InnoDB;`

	_, err = db.ExecContext(ctx, createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	return nil
}

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

func (mi *MySQLImpl) databaseInfo(ctx context.Context, dsn, namespace, name string) (DatabaseInfo, error) {
	log.FromContext(ctx).Info("creating a username and database in the dbaas_controller database")

	// MySQL username must use valid characters and be at most 16 characters long
	const (
		maxUsernameLength     = 16
		maxPasswordLength     = 24
		maxDatabaseNameLength = 64
	)

	info := DatabaseInfo{}

	// Connect to MySQL server and select the dbaas_controller database
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return info, fmt.Errorf("failed to connect to MySQL server: %w", err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, "USE dbaas_controller")
	if err != nil {
		return info, fmt.Errorf("failed to select database: %w", err)
	}

	// Check if the username and database name already exist
	// Use prepared statements to avoid SQL injection vulnerabilities.
	err = db.QueryRowContext(
		ctx, "SELECT username, password, dbname FROM users WHERE name = ? AND namespace = ?", name, namespace).Scan(
		&info.Username,
		&info.Password,
		&info.Dbname,
	)
	if err != nil {
		// check if the error is a not found error
		if err == sql.ErrNoRows {
			info.Username = generateRandomString(maxUsernameLength)
			info.Password = generateRandomString(maxPasswordLength)

			dbnamePrefix := namespace
			if len(dbnamePrefix) > 50 {
				dbnamePrefix = dbnamePrefix[:50]
			}

			info.Dbname = fmt.Sprintf(
				"%s_%s", dbnamePrefix, generateRandomString(maxDatabaseNameLength-len(dbnamePrefix)-1))

			// Insert the user into the users table
			_, err = db.ExecContext(
				ctx, "INSERT INTO users (name, namespace, username, password, dbname) VALUES (?, ?, ?, ?, ?)",
				name, namespace, info.Username, info.Password, info.Dbname)
			if err != nil {
				return info, fmt.Errorf("failed to insert user into users table: %w", err)
			}
		} else {
			return info, fmt.Errorf("failed to query users table: %w", err)
		}
	}
	return info, nil
}

// CreateDatabase creates a database in the MySQL database if it does not exist.
// It also creates a user and grants the user permissions on the database.
// This function is idempotent and can be called multiple times without side effects.
func (mi *MySQLImpl) CreateDatabase(ctx context.Context, dsn, name, namespace string) (DatabaseInfo, error) {
	log.FromContext(ctx).Info("Creating MySQL database", "name", name, "namespace", namespace)

	info, err := mi.databaseInfo(ctx, dsn, namespace, name)
	if err != nil {
		return info, fmt.Errorf("failed to get database info: %w", err)
	}
	// Connect to the database server
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return info, fmt.Errorf("create database error connecting to the database server: %w", err)
	}
	defer db.Close()

	// Ping the database to verify connection establishment.
	if err := db.PingContext(ctx); err != nil {
		return info, fmt.Errorf(
			"create database error verifying connection to the database server: %w", err)
	}

	// Create the database
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", info.Dbname))
	if err != nil {
		return info, fmt.Errorf(
			"create database error in creating the database `%s`: %w", info.Dbname, err)
	}

	// Create the user and grant permissions
	// Use prepared statements to avoid SQL injection vulnerabilities.
	_, err = db.ExecContext(
		ctx, fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", info.Username, info.Password))
	if err != nil {
		return info, fmt.Errorf("create database error creating user `%s`: %w", info.Username, err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, "+
			"INDEX, ALTER, CREATE TEMPORARY TABLES, LOCK TABLES, "+
			"EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `%s`.* TO '%s'@'%%'",
		info.Dbname, info.Username))
	if err != nil {
		return info, fmt.Errorf(
			"create database error granting privileges to user `%s` on database `%s`: %w", info.Username, info.Dbname, err)
	}

	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		return info, fmt.Errorf("create database error flushing privileges: %w", err)
	}

	return info, nil
}

func (mi *MySQLImpl) DropDatabase(ctx context.Context, dsn, name, namespace string) error {
	log.FromContext(ctx).Info("Dropping MySQL database", "name", name, "namespace", namespace)

	info, err := mi.databaseInfo(ctx, dsn, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get database info: %w", err)
	}

	// Connect to the database server
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("drop database error connecting to the database server: %w", err)
	}
	defer db.Close()

	// Ping the database to verify connection establishment.
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("drop database error verifying connection to the database server: %w", err)
	}

	// Drop the database
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", info.Dbname))
	if err != nil {
		if err == sql.ErrNoRows {
			log.FromContext(ctx).Info("Database does not exist", "name", name, "namespace", namespace, "dbname", info.Dbname)
		} else {
			return fmt.Errorf("drop database error in dropping the database `%s`: %w", info.Dbname, err)
		}
	} else {
		log.FromContext(ctx).Info("Dropped database", "name", name, "namespace", namespace, "dbname", info.Dbname)
	}

	// Delete the user
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", info.Username))
	if err != nil {
		if err == sql.ErrNoRows {
			log.FromContext(ctx).Info("User does not exist", "name", name, "namespace", namespace, "username", info.Username)
			return nil
		} else {
			return fmt.Errorf("drop database error in dropping user `%s`: %w", info.Username, err)
		}
	} else {
		log.FromContext(ctx).Info("Dropped user", "name", name, "namespace", namespace, "username", info.Username)
	}

	return nil
}

// GetDatabase returns the database name, username, and password for the given name and namespace.
func (mi *MySQLImpl) GetDatabase(ctx context.Context, dsn, name, namespace string) (DatabaseInfo, error) {
	log.FromContext(ctx).Info("Getting MySQL database", "name", name, "namespace", namespace)

	info, err := mi.databaseInfo(ctx, dsn, namespace, name)
	if err != nil {
		return info, fmt.Errorf("failed to get database info: %w", err)
	}

	return info, nil
}
