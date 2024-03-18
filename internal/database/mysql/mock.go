package mysql

import "context"

var _ MySQLInterface = (*MockMySQLer)(nil)

type MockMySQLer struct{}

func (mi *MockMySQLer) Ping(ctx context.Context, dsn string) error {
	return nil
}

func (mi *MockMySQLer) Version(ctx context.Context, dsn string) (string, error) {
	return "5.7.34", nil
}

func (mi *MockMySQLer) CreateDatabase(ctx context.Context, dsn, databaseName, userName, password string) error {
	return nil
}
