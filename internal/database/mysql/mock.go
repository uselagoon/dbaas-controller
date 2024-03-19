package mysql

import "context"

// Make sure MySQLMock implements MySQLInterface
var _ MySQLInterface = (*MySQLMock)(nil)

// MySQLMock is a mock implementation of the MySQL database
type MySQLMock struct{}

// Ping pings the MySQL database
func (mi *MySQLMock) Ping(ctx context.Context, dsn string) error {
	return nil
}

// Version returns the version of the MySQL database
func (mi *MySQLMock) Version(ctx context.Context, dsn string) (string, error) {
	return "5.7.34", nil
}

// CreateDatabase creates a database in the MySQL database if it does not exist.
func (mi *MySQLMock) CreateDatabase(ctx context.Context, dsn, databaseName, userName, password string) error {
	return nil
}
