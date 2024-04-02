package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"

	_ "github.com/go-sql-driver/mysql"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

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
	CreateDatabase(ctx context.Context, dsn, name, namespace string) error

	// DropDatabase drops a database in the MySQL database if it exists.
	// This function is idempotent and can be called multiple times without side effects.
	DropDatabase(ctx context.Context, dsn, name, namespace string) error

	// GetDatabase returns the database name, username, and password for the given name and namespace.
	GetDatabase(ctx context.Context, dsn, name, namespace string) (string, string, string, error)
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

type database struct {
	name      string
	namespace string
	username  string
	password  string
	dbname    string
}

func (mi *MySQLImpl) databaseInfo(ctx context.Context, dsn, namespace, name string) (database, error) {
	log.FromContext(ctx).Info("creating a username and database in the dbaas_controller database")

	// MySQL username must use valid characters and be at most 16 characters long
	const maxUsernameLength = 16
	const maxPasswordLength = 24
	const maxDatabaseNameLength = 64

	database := database{
		name:      name,
		namespace: namespace,
	}

	// Connect to MySQL server and select the dbaas_controller database
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return database, fmt.Errorf("failed to connect to MySQL server: %w", err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, "USE dbaas_controller")
	if err != nil {
		return database, fmt.Errorf("failed to select database: %w", err)
	}

	// Check if the username and database name already exist
	// Use prepared statements to avoid SQL injection vulnerabilities.
	err = db.QueryRowContext(ctx, "SELECT username, password, dbname FROM users WHERE name = ? AND namespace = ?", name, namespace).Scan(
		&database.username,
		&database.password,
		&database.dbname,
	)
	if err != nil {
		// check if the error is a not found error
		if err == sql.ErrNoRows {
			database.username = generateRandomString(maxUsernameLength)
			database.password = generateRandomString(maxPasswordLength)
			database.dbname = generateRandomString(maxDatabaseNameLength)

			// Insert the user into the users table
			_, err = db.ExecContext(ctx, "INSERT INTO users (name, namespace, username, password, dbname) VALUES (?, ?, ?, ?, ?)",
				name, namespace, database.username, database.password, database.dbname)
			if err != nil {
				return database, fmt.Errorf("failed to insert user into users table: %w", err)
			}
		} else {
			return database, fmt.Errorf("failed to query users table: %w", err)
		}
	}
	return database, nil
}

// CreateDatabase creates a database in the MySQL database if it does not exist.
// It also creates a user and grants the user permissions on the database.
// This function is idempotent and can be called multiple times without side effects.
func (mi *MySQLImpl) CreateDatabase(ctx context.Context, dsn, name, namespace string) error {
	log.FromContext(ctx).Info("Creating MySQL database", "name", name, "namespace", namespace)

	database, err := mi.databaseInfo(ctx, dsn, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get database info: %w", err)
	}
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
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", database.dbname))
	if err != nil {
		return fmt.Errorf("create database error in creating the database `%s`: %w", database.dbname, err)
	}

	// Create the user and grant permissions
	// Use prepared statements to avoid SQL injection vulnerabilities.
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY ?", database.username), database.password)
	if err != nil {
		return fmt.Errorf("create database error creating user `%s`: %w", database.username, err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, LOCK TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `%s`.* TO '%s'@'%%'", database.dbname, database.username))
	if err != nil {
		return fmt.Errorf(
			"create database error granting privileges to user `%s` on database `%s`: %w", database.name, database.dbname, err)
	}

	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		return fmt.Errorf("create database error flushing privileges: %w", err)
	}

	return nil
}

func (mi *MySQLImpl) DropDatabase(ctx context.Context, dsn, name, namespace string) error {
	log.FromContext(ctx).Info("Dropping MySQL database", "name", name, "namespace", namespace)

	database, err := mi.databaseInfo(ctx, dsn, namespace, name)
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
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", database.dbname))
	if err != nil {
		if err == sql.ErrNoRows {
			log.FromContext(ctx).Info("Database does not exist", "name", name, "namespace", namespace, "dbname", database.dbname)
		} else {
			return fmt.Errorf("drop database error in dropping the database `%s`: %w", database.dbname, err)
		}
	} else {
		log.FromContext(ctx).Info("Dropped database", "name", name, "namespace", namespace, "dbname", database.dbname)
	}

	// Delete the user
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", database.username))
	if err != nil {
		if err == sql.ErrNoRows {
			log.FromContext(ctx).Info("User does not exist", "name", name, "namespace", namespace, "username", database.username)
			return nil
		} else {
			return fmt.Errorf("drop database error in dropping user `%s`: %w", database.username, err)
		}
	} else {
		log.FromContext(ctx).Info("Dropped user", "name", name, "namespace", namespace, "username", database.username)
	}

	return nil
}

// GetDatabase returns the database name, username, and password for the given name and namespace.
func (mi *MySQLImpl) GetDatabase(ctx context.Context, dsn, name, namespace string) (string, string, string, error) {
	log.FromContext(ctx).Info("Getting MySQL database", "name", name, "namespace", namespace)

	database, err := mi.databaseInfo(ctx, dsn, namespace, name)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get database info: %w", err)
	}

	return database.dbname, database.username, database.password, nil
}
