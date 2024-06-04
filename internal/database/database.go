package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"

	_ "github.com/go-sql-driver/mysql"
	md "github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// maxUsernameLength MySQL and PostgreSQL username must use valid characters and be at most 16 characters long
	maxUsernameLength = 16
	// maxPasswordLength MySQL and PostgreSQL password must use valid characters and be at most 24 characters long
	maxPasswordLength = 24
	// maxDatabaseNameLength MySQL and PostgreSQL database name must use valid characters and be at most 63 characters long
	maxDatabaseNameLength = 63
	// mysql is the type for MySQL
	mysql = "mysql"
	// postgres is the type for PostgreSQL
	postgres = "postgres"
)

// RelationalDatabaseInfo contains the username, password, and database name of a relational database
type RelationalDatabaseInfo struct {
	// Username is the username for the database
	Username string
	// Password is the password for the database
	Password string
	// Dbname is the database name
	Dbname string
}

// RelationalDatabaseInterface is the interface for a relational database
// Note that the implementation of this interface should be idempotent.
type RelationalDatabaseInterface interface {
	// GetConnection returns a connection to the relational database
	GetConnection(ctx context.Context, dsn string, dbType string) (*sql.DB, error)

	// Ping pings the relational database
	Ping(ctx context.Context, dsn string, dbType string) error

	// Version returns the version of the relational database
	Version(ctx context.Context, dsn string, dbType string) (string, error)

	// Load of the database measured in MB of data and index size.
	// Higher values indicate more data and indexes.
	Load(ctx context.Context, dsn string, dbType string) (int, error)

	// Initialize initializes the relational database
	// This is used by the database {MySQL,PostgreSQL} provider to initialize the relational database.
	// It does setup the dbass_controller database.
	// This function is idempotent and can be called multiple times without side effects.
	Initialize(ctx context.Context, dsn string, dbType string) error

	// CreateDatabase creates a database in the relational database if it does not exist.
	// It also creates a user and grants the user permissions on the database.
	// This function is idempotent and can be called multiple times without side effects.
	// returns the database name, username, and password
	CreateDatabase(ctx context.Context, dsn, name, namespace, dbType string) (RelationalDatabaseInfo, error)

	// DropDatabase drops a database in the MySQL or PostgreSQL database if it exists.
	// This function is idempotent and can be called multiple times without side effects.
	DropDatabase(ctx context.Context, dsn, name, namespace, dbType string) error

	// GetDatabaseInfo returns the database name, username, and password for the given name and namespace.
	GetDatabaseInfo(ctx context.Context, dsn, name, namespace, dbType string) (RelationalDatabaseInfo, error)

	// SetDatabaseInfo sets the database name, username, and password for the given name and namespace.
	SetDatabaseInfo(ctx context.Context, dsn, name, namespace, dbType string, info RelationalDatabaseInfo) error
}

// RelationalDatabaseImpl is the implementation of the RelationalDatabaseInterface
type RelationalDatabaseImpl struct {
	connectionCache map[string]*sql.DB
}

// Make sure RelationalDatabaseBasicImpl implements RelationalDatabaseBasicInterface
var _ RelationalDatabaseInterface = (*RelationalDatabaseImpl)(nil)

// NewRelationalDatabaseBasicImpl creates a new RelationalDatabaseBasicImpl
func New() *RelationalDatabaseImpl {
	return &RelationalDatabaseImpl{
		connectionCache: make(map[string]*sql.DB),
	}
}

// GetConnection returns a connection to the MySQL or PostgreSQL database
func (ri *RelationalDatabaseImpl) GetConnection(ctx context.Context, dsn string, dbType string) (*sql.DB, error) {
	if db, ok := ri.connectionCache[dsn]; ok {
		return db, nil
	}

	log.FromContext(ctx).Info("Opening new database connection", "dbType", dbType)
	db, err := sql.Open(dbType, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s database: %w", dbType, err)
	}

	ri.connectionCache[dsn] = db
	return db, nil
}

// Ping pings the relational database
func (ri *RelationalDatabaseImpl) Ping(ctx context.Context, dsn string, dbType string) error {
	log.FromContext(ctx).Info("Pinging database", "dbType", dbType)
	db, err := ri.GetConnection(ctx, dsn, dbType)
	if err != nil {
		return fmt.Errorf("ping failed to open %s database: %w", dbType, err)
	}

	if err := db.PingContext(ctx); err != nil {
		if dbType == mysql {
			log.FromContext(ctx).Error(err, "Failed to ping MMMMMySQL database")
			var driverErr *md.MySQLError
			if errors.As(err, &driverErr) {
				switch driverErr.Number {
				case 1044, 1045:
					return fmt.Errorf("failed to ping %s database due to invalid credentials: %w", dbType, err)
				case 1049:
					return fmt.Errorf("failed to ping %s database due to database does not exist: %w", dbType, err)
				default:
					return fmt.Errorf("failed to ping driveErr %s database: %w", dbType, err)
				}
			}
		} else if dbType == postgres {
			var driverErr *pq.Error
			if errors.As(err, &driverErr) {
				switch driverErr.Code {
				case "28P01":
					return fmt.Errorf("failed to ping %s database due to invalid credentials: %w", dbType, err)
				case "3D000":
					return fmt.Errorf("failed to ping %s database due to database does not exist: %w", dbType, err)
				default:
					return fmt.Errorf("failed to ping %s database: %w", dbType, err)
				}
			}
		}
		return fmt.Errorf("failed to ping %s database: %w", dbType, err)
	}

	return nil
}

// Version returns the version of the MySQL or PostgreSQL database
func (ri *RelationalDatabaseImpl) Version(ctx context.Context, dsn string, dbType string) (string, error) {
	log.FromContext(ctx).Info("Getting database version", "dbType", dbType)
	db, err := ri.GetConnection(ctx, dsn, dbType)
	if err != nil {
		return "", fmt.Errorf("version failed to open %s database: %w", dbType, err)
	}

	var version string
	err = db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("version failed to get %s database version: %w", dbType, err)
	}

	return version, nil
}

// Load returns the load of the MySQL or PostgreSQL database measured in MB of data and index size.
// Note it doesn't include CPU or memory usage which could be obtained from other sources.
func (ri *RelationalDatabaseImpl) Load(ctx context.Context, dsn string, dbType string) (int, error) {
	log.FromContext(ctx).Info("Getting database load", "dbType", dbType)
	db, err := ri.GetConnection(ctx, dsn, dbType)
	if err != nil {
		return 0, fmt.Errorf("load failed to open %s database: %w", dbType, err)
	}

	var totalLoad float64
	if dbType == mysql {
		err = db.QueryRowContext(ctx, "SELECT data_length + index_length FROM information_schema.tables").Scan(&totalLoad)
		if err != nil {
			return 0, fmt.Errorf("load failed to get %s database load: %w", dbType, err)
		}
	} else if dbType == postgres {
		err = db.QueryRowContext(ctx, "SELECT pg_database_size(current_database())").Scan(&totalLoad)
		if err != nil {
			return 0, fmt.Errorf("load failed to get %s database load: %w", dbType, err)
		}
	} else {
		return 0, fmt.Errorf("load failed to get %s database load: unsupported dbType", dbType)
	}
	// convert bytes to MB
	totalLoadMB := totalLoad / 1024 / 1024
	return int(totalLoadMB), nil
}

// Initialize initializes the MySQL or PostgreSQL database
// This is used by the database {MySQL,PostgreSQL} provider to initialize the MySQL or PostgreSQL database.
// It does setup the dbass_controller database.
// This function is idempotent and can be called multiple times without side effects.
func (ri *RelationalDatabaseImpl) Initialize(ctx context.Context, dsn string, dbType string) error {
	log.FromContext(ctx).Info("Initializing database", "dbType", dbType)
	db, err := ri.GetConnection(ctx, dsn, dbType)
	if err != nil {
		return fmt.Errorf("initialize failed to open %s database: %w", dbType, err)
	}

	if dbType == mysql {
		_, err = db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS dbaas_controller")
		if err != nil {
			return fmt.Errorf("initialize failed to create %s database: %w", dbType, err)
		}

		_, err = db.ExecContext(ctx, "USE dbaas_controller")
		if err != nil {
			return fmt.Errorf("initialize failed to use %s database: %w", dbType, err)
		}

		_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			namespace VARCHAR(255) NOT NULL,
			username VARCHAR(16) NOT NULL UNIQUE,
			password VARCHAR(255) NOT NULL,
			dbname VARCHAR(255) NOT NULL UNIQUE,
			CONSTRAINT unique_name_namespace UNIQUE (name, namespace)
		) ENGINE=InnoDB`)
		if err != nil {
			return fmt.Errorf("initialize failed to create %s table: %w", dbType, err)
		}
	} else if dbType == postgres {
		_, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS dbaas_controller")
		if err != nil {
			return fmt.Errorf("initialize failed to create %s database: %w", dbType, err)
		}

		_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS dbaas_controller.users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			namespace VARCHAR(255) NOT NULL,
			username VARCHAR(16) NOT NULL UNIQUE,
			password VARCHAR(255) NOT NULL,
			dbname VARCHAR(255) NOT NULL UNIQUE,
			CONSTRAINT unique_name_namespace UNIQUE (name, namespace)
		)`)
		if err != nil {
			return fmt.Errorf("initialize failed to create %s table: %w", dbType, err)
		}
	} else {
		return fmt.Errorf("initialize failed to initialize %s database: unsupported dbType", dbType)
	}

	return nil
}

// CreateDatabase creates a database in the MySQL or PostgreSQL server if it does not exist.
func (ri *RelationalDatabaseImpl) CreateDatabase(
	ctx context.Context,
	dsn, name, namespace string,
	dbType string,
) (RelationalDatabaseInfo, error) {
	log.FromContext(ctx).Info("Creating database", "dbType", dbType)
	db, err := ri.GetConnection(ctx, dsn, dbType)
	if err != nil {
		return RelationalDatabaseInfo{}, fmt.Errorf("create database failed to open %s database: %w", dbType, err)
	}

	var info RelationalDatabaseInfo
	if dbType == mysql {
		info, err = ri.databaseInfoMySQL(ctx, dsn, name, namespace)
		if err != nil {
			return info, fmt.Errorf("create %s database failed to get database info: %w", dbType, err)
		}
		// Create the database
		_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", info.Dbname))
		if err != nil {
			return info, fmt.Errorf(
				"create %s database error in creating the database `%s`: %w", dbType, info.Dbname, err)
		}
		// Create the user and grant permissions
		// Use prepared statements to avoid SQL injection vulnerabilities.
		_, err = db.ExecContext(
			ctx, fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", info.Username, info.Password))
		if err != nil {
			return info, fmt.Errorf("create %s database error creating user `%s`: %w", dbType, info.Username, err)
		}

		_, err = db.ExecContext(ctx, fmt.Sprintf(
			"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, "+
				"INDEX, ALTER, CREATE TEMPORARY TABLES, LOCK TABLES, "+
				"EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `%s`.* TO '%s'@'%%'",
			info.Dbname, info.Username))
		if err != nil {
			return info, fmt.Errorf(
				"create %s database error granting privileges to user `%s` on database `%s`: %w",
				dbType, info.Username, info.Dbname, err)
		}

		_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
		if err != nil {
			return info, fmt.Errorf("create %s database error flushing privileges: %w", dbType, err)
		}
	} else if dbType == postgres {
		info, err = ri.databaseInfoPostgreSQL(ctx, dsn, name, namespace)
		if err != nil {
			return info, fmt.Errorf("create database failed to get %s database info: %w", dbType, err)
		}
		// Create the database
		_, err = db.Exec(fmt.Sprintf("CREATE DATABASE \"%s\"", info.Dbname))
		if pqErr, ok := err.(*pq.Error); !ok || ok && pqErr.Code != "42P04" {
			// either the error is not a pq.Error or it is a pq.Error but not a duplicate_database error
			// 42P04 is the error code for duplicate_database
			return info, fmt.Errorf(
				"create %s database error in creating the database `%s`: %w", dbType, info.Dbname, err)
		}

		// Check if user exists and create or update the user
		var userExists int
		err = db.QueryRow(fmt.Sprintf("SELECT 1 FROM pg_roles WHERE rolname='%s'", info.Username)).Scan(&userExists)
		if err != nil && err != sql.ErrNoRows {
			return info, fmt.Errorf(
				"create %s database error in check if user exists in database `%s`: %w", dbType, info.Dbname, err)
		}

		if userExists == 0 {
			// Create the user with encrypted password
			_, err = db.Exec(fmt.Sprintf("CREATE USER \"%s\" WITH ENCRYPTED PASSWORD '%s'", info.Username, info.Password))
			if err != nil {
				return info, fmt.Errorf(
					"create %s database error in create user in database `%s`: %w", dbType, info.Dbname, err)
			}
		}

		// Grant privileges
		_, err = db.Exec(fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE \"%s\" TO \"%s\"", info.Dbname, info.Username))
		if err != nil {
			return info, fmt.Errorf(
				"create %s database error in grant privileges in database `%s`: %w", dbType, info.Dbname, err)
		}
	} else {
		return RelationalDatabaseInfo{}, fmt.Errorf(
			"create database failed to create %s database: unsupported dbType", dbType)
	}

	return info, nil
}

// DropDatabase drops a database in the MySQL or PostgreSQL database if it exists.
func (ri *RelationalDatabaseImpl) DropDatabase(ctx context.Context, dsn, name, namespace, dbType string) error {
	log.FromContext(ctx).Info("Dropping database", "dbType", dbType)
	db, err := ri.GetConnection(ctx, dsn, dbType)
	if err != nil {
		return fmt.Errorf("drop database failed to open %s database: %w", dbType, err)
	}

	info := RelationalDatabaseInfo{}
	if dbType == mysql {
		info, err = ri.databaseInfoMySQL(ctx, dsn, name, namespace)
		if err != nil {
			return fmt.Errorf("drop database failed to get database info: %w", err)
		}
		// Drop the database
		_, err = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", info.Dbname))
		if err != nil {
			return fmt.Errorf("drop database failed to drop %s database: %w", dbType, err)
		}
		// Drop the user
		_, err = db.ExecContext(ctx, fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", info.Username))
		if err != nil {
			return fmt.Errorf("drop database failed to drop user: %w", err)
		}
		// flush privileges
		_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
		if err != nil {
			return fmt.Errorf("drop database failed to flush privileges: %w", err)
		}
	} else if dbType == postgres {
		info, err = ri.databaseInfoPostgreSQL(ctx, dsn, name, namespace)
		if err != nil {
			return fmt.Errorf("drop database failed to get database info: %w", err)
		}
		// Disconnect all users from the database
		_, err = db.Exec(
			fmt.Sprintf(
				"SELECT pg_terminate_backend(pg_stat_activity.pid) FROM pg_stat_activity WHERE pg_stat_activity.datname = '%s' AND pid <> pg_backend_pid()", // nolint: lll
				info.Dbname,
			),
		)
		if err != nil {
			return fmt.Errorf("drop database failed to disconnect users from %s database: %w", dbType, err)
		}
		// Drop the database
		_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS \"%s\"", info.Dbname))
		if err != nil {
			return fmt.Errorf("drop database failed to drop %s database: %w", dbType, err)
		}
		// Drop the user
		_, err = db.Exec(fmt.Sprintf("DROP USER IF EXISTS \"%s\"", info.Username))
		if err != nil {
			return fmt.Errorf("drop database failed to drop user: %w", err)
		}
	} else {
		return fmt.Errorf("drop database failed to drop %s database: unsupported dbType", dbType)
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

// createUserInfo creates a random username, password, and database name
func createUserInfo(_ context.Context, namespace string) RelationalDatabaseInfo {
	info := RelationalDatabaseInfo{
		Username: generateRandomString(maxUsernameLength),
		Password: generateRandomString(maxPasswordLength),
	}

	dbnamePrefix := namespace
	if len(dbnamePrefix) > 50 {
		dbnamePrefix = dbnamePrefix[:50]
	}

	info.Dbname = fmt.Sprintf(
		"%s_%s", dbnamePrefix, generateRandomString(maxDatabaseNameLength-len(dbnamePrefix)-1))

	return info
}

// databaseInfoMySQL returns the username, password, and database name for the given name and namespace.
// It also creates the user and database if they do not exist.
// This function is idempotent and can be called multiple times without side effects.
func (ri *RelationalDatabaseImpl) databaseInfoMySQL(
	ctx context.Context,
	dsn, name, namespace string,
) (RelationalDatabaseInfo, error) {
	var info RelationalDatabaseInfo

	db, err := ri.GetConnection(ctx, dsn, mysql)
	if err != nil {
		return RelationalDatabaseInfo{}, fmt.Errorf("create database failed to open %s database: %w", mysql, err)
	}

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
		if err != sql.ErrNoRows {
			return info, fmt.Errorf("failed %s to query users table: %w", mysql, err)
		}

		info = createUserInfo(ctx, namespace)
		// Insert the user into the users table
		_, err = db.ExecContext(
			ctx, "INSERT INTO users (name, namespace, username, password, dbname) VALUES (?, ?, ?, ?, ?)",
			name, namespace, info.Username, info.Password, info.Dbname)
		if err != nil {
			return info, fmt.Errorf("failed to insert user into users table: %w", err)
		}
	}
	return info, nil
}

// insertUserinfoIntoMysql inserts the user into the users table
// This function is idempotent and can be called multiple times without side effects.
func (ri *RelationalDatabaseImpl) insertUserInfoIntoMysql(
	ctx context.Context,
	dsn, name, namespace string,
	info RelationalDatabaseInfo,
) error {
	db, err := ri.GetConnection(ctx, dsn, mysql)
	if err != nil {
		return fmt.Errorf("set database info failed to open %s database: %w", mysql, err)
	}
	_, err = db.ExecContext(ctx, "USE dbaas_controller")
	if err != nil {
		return fmt.Errorf("failed to select database: %w", err)
	}
	// Insert the user into the users table
	_, err = db.ExecContext(
		ctx, "INSERT IGNORE INTO users (name, namespace, username, password, dbname) VALUES (?, ?, ?, ?, ?)",
		name, namespace, info.Username, info.Password, info.Dbname)
	if err != nil {
		return fmt.Errorf("failed to insert user into users table: %w", err)
	}
	return nil
}

// databaseInfoPostgreSQL returns the username, password, and database name for the given name and namespace.
// It also creates the user and database if they do not exist.
// This function is idempotent and can be called multiple times without side effects.
func (ri *RelationalDatabaseImpl) databaseInfoPostgreSQL(
	ctx context.Context,
	dsn, name, namespace string,
) (RelationalDatabaseInfo, error) {
	var info RelationalDatabaseInfo

	db, err := ri.GetConnection(ctx, dsn, postgres)
	if err != nil {
		return RelationalDatabaseInfo{}, fmt.Errorf("create database failed to open %s database: %w", postgres, err)
	}

	// select username, password and dbname from the users table
	err = db.QueryRowContext(
		ctx, "SELECT username, password, dbname FROM dbaas_controller.users WHERE name = $1 AND namespace = $2",
		name, namespace).Scan(
		&info.Username,
		&info.Password,
		&info.Dbname,
	)
	if err != nil {
		// check if the error is a not found error
		if err != sql.ErrNoRows {
			return info, fmt.Errorf("failed %s to query users table: %w", postgres, err)
		}
		info = createUserInfo(ctx, namespace)
		// Insert the user into the users table
		_, err = db.ExecContext(
			ctx, "INSERT INTO dbaas_controller.users (name, namespace, username, password, dbname) VALUES ($1, $2, $3, $4, $5)",
			name, namespace, info.Username, info.Password, info.Dbname)
		if err != nil {
			return info, fmt.Errorf("failed to insert user into users table: %w", err)
		}
	}

	return info, nil
}

// insertUserInfoIntoPostgreSQL inserts the user into the users table
// This function is idempotent and can be called multiple times without side effects.
func (ri *RelationalDatabaseImpl) insertUserInfoIntoPostgreSQL(
	ctx context.Context,
	dsn, name, namespace string,
	info RelationalDatabaseInfo,
) error {
	db, err := ri.GetConnection(ctx, dsn, postgres)
	if err != nil {
		return fmt.Errorf("set database info failed to open %s database: %w", postgres, err)
	}
	// Insert the user into the users table
	_, err = db.ExecContext(
		ctx, "INSERT INTO dbaas_controller.users (name, namespace, username, password, dbname) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING",
		name, namespace, info.Username, info.Password, info.Dbname)
	if err != nil {
		return fmt.Errorf("failed to insert user into users table: %w", err)
	}
	return nil
}

// GetDatabaseInfo returns the database name, username, and password for the given name and namespace.
func (ri *RelationalDatabaseImpl) GetDatabaseInfo(
	ctx context.Context,
	dsn, name, namespace, dbType string,
) (RelationalDatabaseInfo, error) {
	log.FromContext(ctx).Info("Getting database", "dbType", dbType, "name", name, "namespace", namespace)
	if dbType == "mysql" {
		return ri.databaseInfoMySQL(ctx, dsn, name, namespace)
	} else if dbType == "postgres" {
		return ri.databaseInfoPostgreSQL(ctx, dsn, name, namespace)
	}
	return RelationalDatabaseInfo{}, fmt.Errorf("get database failed to get %s database: unsupported dbType", dbType)
}

// SetDatabaseInfo sets the database name, username, and password for the given name and namespace.
func (ri *RelationalDatabaseImpl) SetDatabaseInfo(
	ctx context.Context,
	dsn, name, namespace, dbType string,
	info RelationalDatabaseInfo,
) error {
	log.FromContext(ctx).Info("Setting database", "dbType", dbType, "name", name, "namespace", namespace)
	if dbType == "mysql" {
		return ri.insertUserInfoIntoMysql(ctx, dsn, name, namespace, info)
	} else if dbType == "postgres" {
		return ri.insertUserInfoIntoPostgreSQL(ctx, dsn, name, namespace, info)
	}
	return nil
}
