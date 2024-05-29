package database

import (
	"context"
	"database/sql"
)

// Make sure RelationalDatabaseMock implements RelationalDatabaseInterface
var _ RelationalDatabaseInterface = (*RelationalDatabaseMock)(nil)

// RelationalDatabaseMock is a mock implementation of the RelationalDatabase database
type RelationalDatabaseMock struct{}

// Ping pings the relational database
func (mi *RelationalDatabaseMock) Ping(ctx context.Context, dsn string, kind string) error {
	return nil
}

// Version returns the version of the RelationalDatabase database
func (mi *RelationalDatabaseMock) Version(ctx context.Context, dsn string, kind string) (string, error) {
	return "5.7.34", nil
}

func (mi *RelationalDatabaseMock) Initialize(ctx context.Context, dsn string, kind string) error {
	return nil
}

// CreateDatabase creates a database in the relational database if it does not exist.
func (mi *RelationalDatabaseMock) CreateDatabase(
	ctx context.Context, dsn, name, namespace, kind string) (RelationalDatabaseInfo, error) {
	return RelationalDatabaseInfo{Username: "user", Password: "pass", Dbname: "db"}, nil
}

func (mi *RelationalDatabaseMock) DropDatabase(ctx context.Context, dsn, name, namespace, kind string) error {
	return nil
}

func (mi *RelationalDatabaseMock) GetDatabase(
	ctx context.Context, dsn, name, namespace, kind string) (RelationalDatabaseInfo, error) {
	return RelationalDatabaseInfo{Username: "user", Password: "pass", Dbname: "db"}, nil
}

func (mi *RelationalDatabaseMock) Load(ctx context.Context, dsn string, kind string) (int, error) {
	return 10, nil
}

func (mi *RelationalDatabaseMock) GetConnection(ctx context.Context, dsn string, kind string) (*sql.DB, error) {
	return nil, nil
}
