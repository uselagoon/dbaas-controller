package mongodb

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// limits based on
	// - https://docs.mongodb.com/manual/reference/limits/
	// - https://docs.aws.amazon.com/documentdb/latest/developerguide/limits.html#limits-naming_constraints
	// maxUsernameLength mongodb username must use valid characters and be at most 16 characters long
	maxUsernameLength = 16
	// maxPasswordLength mongodb password must use valid characters and be at most 24 characters long
	maxPasswordLength = 24
	// maxDatabaseNameLength mongoDB database name must use valid characters and be at most 63 characters long
	maxDatabaseNameLength = 63
)

// MongoDBInterface interface defines the operations that can be performed on a MongoDB client
type MongoDBInterface interface {
	// GetClient returns a connected MongoDB client from the cache or creates a new one
	GetClient(ctx context.Context, mop MongoDBClientOptions) (*mongo.Client, error)
	// GetUser retrieves a user based on name and namespace
	GetUser(ctx context.Context, mop MongoDBClientOptions, name, namespace string) (MongoDBInfo, error)
	// DropUserAndDatabase removes a user's database, their document in the users collection,
	// and their MongoDB authenticated user account.
	DropUserAndDatabase(ctx context.Context, mop MongoDBClientOptions, name, namespace string) error
	// CreateUserAndDatabase creates or updates a user in the dbaas_controller.users collection and
	// ensures a user database with authentication is created.
	CreateUserAndDatabase(ctx context.Context, mop MongoDBClientOptions, name, namespace string) (MongoDBInfo, error)
	// Version retrieves the version of the MongoDB server.
	Version(ctx context.Context, mop MongoDBClientOptions) (string, error)
	// Ping checks if the MongoDB server is reachable
	Ping(ctx context.Context, mop MongoDBClientOptions) error
}

// MongoDBInfo contains the username, password, and database name of a MongoDB user
type MongoDBInfo struct {
	// Username is the username of the MongoDB user
	Username string
	// Password is the password of the MongoDB user
	Password string
	// Dbname is the name of the MongoDB database
	Dbname string
}

// MongoDBClientOptions contains the options required to connect to a MongoDB server
type MongoDBClientOptions struct {
	Name      string
	Hostname  string
	Port      int
	Username  string
	Password  string
	Mechanism string
	Source    string
	TLS       bool
}

// clientOptions returns the options required to connect to a MongoDB server
func (mco *MongoDBClientOptions) clientOptions() *options.ClientOptions {
	credential := options.Credential{
		AuthSource:    mco.Source,
		Username:      mco.Username,
		Password:      mco.Password,
		AuthMechanism: mco.Mechanism,
	}
	uri := fmt.Sprintf("mongodb://%s:%d", mco.Hostname, mco.Port)
	clientOpts := options.Client().ApplyURI(uri).
		SetAuth(credential)
	if mco.TLS {
		clientOpts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}
	return clientOpts
}

// MongoDBImpl encapsulates the MongoDB client and operations
type MongoDBImpl struct {
	connectionCache map[MongoDBClientOptions]*mongo.Client
}

// Make sure MongoDBImpl implements MongoDBInterface
var _ MongoDBInterface = (*MongoDBImpl)(nil)

// NewMongoDBImpl creates a new instance of MongoDBImpl with initialized properties
func NewMongoDBImpl() *MongoDBImpl {
	return &MongoDBImpl{
		connectionCache: make(map[MongoDBClientOptions]*mongo.Client),
	}
}

// GetClient returns a connected MongoDB client from the cache or creates a new one
func (mi *MongoDBImpl) GetClient(ctx context.Context, mop MongoDBClientOptions) (*mongo.Client, error) {
	if client, ok := mi.connectionCache[mop]; ok {
		return client, nil
	}

	client, err := mongo.Connect(ctx, mop.clientOptions())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	mi.connectionCache[mop] = client
	return client, nil
}

// GetUser retrieves a user based on name and namespace
func (mi *MongoDBImpl) GetUser(ctx context.Context, mop MongoDBClientOptions, name, namespace string) (MongoDBInfo, error) {
	client, err := mi.GetClient(ctx, mop)
	if err != nil {
		return MongoDBInfo{}, fmt.Errorf("failed to get MongoDB User: %w", err)
	}
	collection := client.Database("dbaas_controller").Collection("users")
	var result MongoDBInfo

	filter := bson.M{"name": name, "namespace": namespace}
	if err = collection.FindOne(ctx, filter).Decode(&result); err != nil {
		if err == mongo.ErrNoDocuments {
			return MongoDBInfo{}, fmt.Errorf("user %s in namespace %s does not exist, %w", name, namespace, err)
		}
		return MongoDBInfo{}, fmt.Errorf("failed to retrieve user: %w", err)
	}

	return result, nil
}

func createMongoDBUserInfo(namespace string) MongoDBInfo {
	username := generateRandomString(maxUsernameLength)
	password := generateRandomString(maxPasswordLength)
	dbname := fmt.Sprintf("%s_%s", namespace, generateRandomString(maxDatabaseNameLength-len(namespace)-1))

	return MongoDBInfo{
		Username: username,
		Password: password,
		Dbname:   dbname,
	}
}

// DropUserAndDatabase removes a user's database, their document in the users collection,
// and their MongoDB authenticated user account.
func (mi *MongoDBImpl) DropUserAndDatabase(ctx context.Context, mop MongoDBClientOptions, name, namespace string) error {
	// Retrieve user information to get the database name and username
	log.FromContext(ctx).Info("Drop user and database", "name", name, "namespace", namespace)
	client, err := mi.GetClient(ctx, mop)
	if err != nil {
		return fmt.Errorf("failed to drop user and database: %w", err)
	}
	userInfo, err := mi.GetUser(ctx, mop, name, namespace)
	if err != nil {
		// check if the error is mongo.ErrNoDocuments
		if errors.Is(err, mongo.ErrNoDocuments) {
			// if the user does not exist then return nil
			log.FromContext(ctx).Info("User does not exist", "name", name, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("failed to retrieve user info: %w", err)
	}

	// Drop the user's specific database
	userDB := client.Database(userInfo.Dbname)
	if err := userDB.Drop(ctx); err != nil {
		return fmt.Errorf("failed to drop user database %s: %w", userInfo.Dbname, err)
	}

	// Drop the MongoDB authenticated user
	adminDB := client.Database("admin")
	command := bson.D{{Key: "dropUser", Value: userInfo.Username}}
	if err := adminDB.RunCommand(ctx, command).Err(); err != nil {
		return fmt.Errorf("failed to drop MongoDB user %s: %w", userInfo.Username, err)
	}

	// Remove the user document from the dbaas_controller.users collection
	collection := client.Database("dbaas_controller").Collection("users")
	filter := bson.M{"name": name, "namespace": namespace}
	result, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete user document: %w", err)
	}
	if result.DeletedCount == 0 {
		return fmt.Errorf("no user document found with name %s in namespace %s", name, namespace)
	}

	return nil
}

// CreateUserAndDatabase creates or updates a user in the dbaas_controller.users collection
// and ensures a user database with authentication is created.
// This function is idempotent.
func (mi *MongoDBImpl) CreateUserAndDatabase(
	ctx context.Context,
	mop MongoDBClientOptions,
	name, namespace string,
) (MongoDBInfo, error) {
	log.FromContext(ctx).Info("Create user and database", "name", name, "namespace", namespace)
	client, err := mi.GetClient(ctx, mop)
	if err != nil {
		return MongoDBInfo{}, fmt.Errorf("failed to create user and database: %w", err)
	}
	// First try to get the user info to see if the user already exists
	userInfo, err := mi.GetUser(ctx, mop, name, namespace)
	if err != nil {
		// check if it is mongo.ErrNoDocuments
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return MongoDBInfo{}, fmt.Errorf("failed to get user info: %w", err)
		} else {
			log.FromContext(ctx).Info("User does not exist", "name", name, "namespace", namespace)
			userInfo = createMongoDBUserInfo(namespace)
		}
	}

	collection := client.Database("dbaas_controller").Collection("users")

	// Ensure the user's specific database exists by inserting a dummy document
	userDB := client.Database(userInfo.Dbname)
	_, err = userDB.Collection("init").InsertOne(ctx, bson.M{"init": "true"})
	if err != nil {
		return MongoDBInfo{}, fmt.Errorf("failed to ensure database exists: %w", err)
	}

	// Create or update the user document in the dbaas_controller.users collection
	filter := bson.M{"name": name, "namespace": namespace}
	update := bson.M{"$setOnInsert": userInfo}
	opts := options.FindOneAndUpdate().SetUpsert(true)
	err = collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&userInfo)
	if err != nil && err != mongo.ErrNoDocuments {
		return MongoDBInfo{}, fmt.Errorf("failed to create or update user document: %w", err)
	}

	// Attempt to create the MongoDB user for authentication
	userExists, err := mi.mongoUserExists(ctx, client, userInfo.Username)
	if err != nil {
		return MongoDBInfo{}, fmt.Errorf("failed to check if user exists: %w", err)
	}
	if userExists {
		log.FromContext(ctx).Info("User already exists", "user", userInfo.Username)
		return userInfo, nil
	}

	adminDB := client.Database("admin")
	command := bson.D{
		{Key: "createUser", Value: userInfo.Username},
		{Key: "pwd", Value: userInfo.Password},
		{Key: "roles", Value: []bson.M{{"role": "readWrite", "db": userInfo.Dbname}}},
	}
	err = adminDB.RunCommand(ctx, command).Err()
	if err != nil {
		return MongoDBInfo{}, fmt.Errorf("failed to create MongoDB user: %w", err)
	}

	return userInfo, nil
}

// CreateMongoDBInfo creates MongoDBInfo with random credentials
func CreateMongoDBInfo(namespace string) MongoDBInfo {
	info := MongoDBInfo{
		Username: generateRandomString(maxUsernameLength),
		Password: generateRandomString(maxPasswordLength),
	}

	dbnamePrefix := namespace
	if len(dbnamePrefix) > 50 {
		dbnamePrefix = dbnamePrefix[:50]
	}

	info.Dbname = fmt.Sprintf("%s_%s", dbnamePrefix, generateRandomString(maxDatabaseNameLength-len(dbnamePrefix)-1))
	return info
}

// Version retrieves the version of the MongoDB server.
func (mi *MongoDBImpl) Version(ctx context.Context, mop MongoDBClientOptions) (string, error) {
	log.FromContext(ctx).Info("Retrieve MongoDB version")
	client, err := mi.GetClient(ctx, mop)
	if err != nil {
		return "", fmt.Errorf("failed to get version: %w", err)
	}
	var result bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to execute buildInfo command: %w", err)
	}

	version, ok := result["version"].(string)
	if !ok {
		return "", fmt.Errorf("version not found in buildInfo response")
	}
	return version, nil
}

// Ping checks if the MongoDB server is reachable
func (mi *MongoDBImpl) Ping(ctx context.Context, mop MongoDBClientOptions) error {
	log.FromContext(ctx).Info("Ping MongoDB server")
	client, err := mi.GetClient(ctx, mop)
	if err != nil {
		return fmt.Errorf("failed to ping MongoDB server: %w", err)
	}
	return client.Ping(ctx, nil)
}

// mongoUserExists checks if a user exists in the MongoDB admin database
// This function is idempotent.
func (mi *MongoDBImpl) mongoUserExists(
	ctx context.Context,
	client *mongo.Client,
	username string,
) (bool, error) {
	log.FromContext(ctx).Info("Check if user exists", "user", username)
	adminDB := client.Database("admin")
	command := bson.D{{Key: "usersInfo", Value: bson.M{"user": username, "db": "admin"}}}

	var result bson.M
	err := adminDB.RunCommand(ctx, command).Decode(&result)
	if err != nil {
		return false, fmt.Errorf("failed to run usersInfo command: %w", err)
	}

	users, ok := result["users"].(primitive.A)
	// Make sure this assertion matches the actual data structure	fmt.Printf("Users: %+v\n", users)
	if !ok || len(users) == 0 {
		return false, nil
	}

	return true, nil
}

// Helper function to generate random strings
func generateRandomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}
