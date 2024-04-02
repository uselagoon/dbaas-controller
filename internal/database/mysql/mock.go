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

func (mi *MySQLMock) Initialize(ctx context.Context, dsn string) error {
	return nil
}

// CreateDatabase creates a database in the MySQL database if it does not exist.
func (mi *MySQLMock) CreateDatabase(ctx context.Context, dsn, name, namespace string) error {
	return nil
}

func (mi *MySQLMock) DropDatabase(ctx context.Context, dsn, name, namespace string) error {
	return nil
}

func (mi *MySQLMock) GetDatabase(ctx context.Context, dsn, name, namespace string) (string, string, string, error) {
	return "user", "pass", "db", nil
}

func (mi *MySQLMock) Load(ctx context.Context, dsn string) (int, error) {
	return 10, nil
}
