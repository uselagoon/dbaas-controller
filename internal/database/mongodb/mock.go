package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
)

// MongoDBMock is a mock implementation of the MongoDB interface
type MongoDBMock struct{}

// Make sure MongoDBImpl implements MongoDBInterface
var _ MongoDBInterface = (*MongoDBMock)(nil)

// GetClient returns a connected MongoDB client from the cache or creates a new one
func (m *MongoDBMock) GetClient(ctx context.Context, mop MongoDBClientOptions) (*mongo.Client, error) {
	return nil, nil
}

// GetUser retrieves a user based on name and namespace
func (m *MongoDBMock) GetUser(
	ctx context.Context,
	mop MongoDBClientOptions,
	name, namespace string,
) (MongoDBInfo, error) {
	return MongoDBInfo{Username: "mock-user", Password: "mock-pw", Dbname: "mock-db"}, nil
}

// DropUserAndDatabase drops a user and database in MongoDB
func (m *MongoDBMock) DropUserAndDatabase(ctx context.Context, mop MongoDBClientOptions, name, namespace string) error {
	return nil
}

// CreateUserAndDatabase creates a new user and database in MongoDB
func (m *MongoDBMock) CreateUserAndDatabase(
	ctx context.Context,
	mop MongoDBClientOptions,
	name, namespace string,
) (MongoDBInfo, error) {
	return MongoDBInfo{Username: "mock-user", Password: "mock-pw", Dbname: "mock-db"}, nil
}

// Version returns the version of the MongoDB server
func (m *MongoDBMock) Version(ctx context.Context, mop MongoDBClientOptions) (string, error) {
	return "4.4.0", nil
}

// Ping checks if the MongoDB server is reachable
func (m *MongoDBMock) Ping(ctx context.Context, mop MongoDBClientOptions) error {
	return nil
}
