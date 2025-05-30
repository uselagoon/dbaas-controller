# DBaaS Controller

[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/10670/badge)](https://www.bestpractices.dev/projects/10670)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/uselagoon/dbaas-controller/badge)](https://securityscorecards.dev/viewer/?uri=github.com/uselagoon/dbaas-controller)

## Overview

The dbaas-controller is designed to be used by Lagoon, specifically focusing on provisioning database access for Lagoon workloads. The dbaas-controller aims to facilitate easier updates, migrations, and overall management of database resources in Lagoon environments.

It allows for provisiong and deprovisioning of shared MySQL/MariaDB, PostgreSQL, and MongoDB databases.

## Current Status of the Project

WIP - Work in Progress
There is still a some work to be done on this project. The current status is that the controller is able to provision and deprovision MySQL databases. But there is still a lot of work to be done to make it production ready.

- [x] Setup e2e tests
- [x] Provision MySQL databases (basic + seeding) - no support for additional users etc.
- [x] Deprovision MySQL databases
- [x] Provision PostgreSQL databases
- [x] Deprovision PostgreSQL databases
- [ ] Provision MongoDB databases
- [ ] Deprovision MongoDB databases
- [ ] Plan to migrate from old `dbaaas-operator` to `dbaas-controller`

## Testing

To run the unit tests, you can use the following command:

```bash
make test
```

To run the end-to-end tests, you can use the following command:

```bash
make test-e2e
```

Note that the end-to-end tests require a kind cluster to be running. Make sure to have kind installed and running before running the tests.

DANGER ZONE: Do not run the end-to-end tests on a production cluster. The tests will create and delete resources in the cluster!!!

## Installation

See [lagoon-charts](https://github.com/uselagoon/lagoon-charts)

## Differences from the Existing Operator

The new dbaas-controller introduces significant changes to the provisioning and management of database resources. It leverages a DatabaseRequest resource for provisioning, which tracks the status of the request. The controller will also support automatic updates to hostnames, reduce reliance on CRDs for data storage, and utilize Kubernetes native items like kind: Secrets and kind: Services.

Key Features:

- Provisioning Resource: Uses DatabaseRequest instead of direct HTTP requests for provisioning databases.
- Automatic Updates: Enables automatic updates to consumer services if a provider's details change.
- Utilizes native Kubernetes secret resources for storing database credentials and connection details.
- Pooling and Disabling Providers: Supports adding new providers to a pool and marking providers as disabled or unable to deprovision.
- Migration Support: Offers mechanisms to migrate consumers between providers seamlessly.
- Able to reusing existing databases by using a seed secret to populate the database with data.

## Custom Resource Definitions

To interact with the dbaas-controller, the following CRDs are introduced:

- RelationalDatabaseProvider
    - This CRD is used to define a database provider, such as MySQL and PostgreSQL.
- DatabaseRequest
- DatabaseMigration

Basic usage of the CRs in combination with the dbaas-controller is outlined below.

- DatabaseRequest: Lagoon creates a DatabaseRequest CR to request a database instance
    - The dbaas-controller processes the request and provisions the database instance based on the request
    - The controller uses the relevant RelationalDatabaseProvider CR to determine how it should provision the database

## RelationalDatabaseProvider CRD Documentation

The `RelationalDatabaseProvider` CRD defines a Kubernetes-native way to manage relational database connections and configurations. This custom resource allows to define MySQL and PostgreSQL databases.

Use the status connectionStatus field to check the status of the MySQL connections defined in the spec.

### RelationalDatabaseProvider Spec Fields

- kind (required): The type of database provider, which can be either mysql or postgresql.
- scope (required): Defines the scope of the database request, which influences the environment setup. Valid values are production, development, and custom. Defaults to development if not specified.
- connections (required): A list of `connection` objects that detail the connection parameters to MySQL or PostgreSQL databases. At least one connection must be defined.

- connection Fields
    - name (required): A unique name for the MySQL or PostgreSQL database connection, used to identify and reference the connection in database requests.
    - hostname (required): The hostname of the MySQL or PostgreSQL database server.
    - replicaHostnames (optional): A list of hostnames for the MySQLi or PostgreSQL replica databases.
    - passwordSecretRef (required): A reference to a Kubernetes Secret containing the password for the database connection.
    - port (required): The port on which the MySQLi or PostgreSQL database server is listening. Must be between 1 and 65535.
    - username (required): The username for logging into the MySQLi or PostgreSQL database.
    - enabled (required): A flag indicating whether this database connection is enabled. Defaults to true.

### RelationalDatabaseProvider Status Fields

- conditions: Provides detailed conditions of the `RelationalDatabaseProvider` like readiness, errors, etc.
- connectionStatus: A list of statuses for the MySQL or PostgreSQL connections defined in the spec.

- connectionStatus Fields
    - hostname (required): The hostname of the MySQL or PostgreSQL database server.
    - mysqlVersion (required): The version of the MySQL or PostgreSQL server.
    - enabled (required): Indicates if the database connection is enabled.
    - status (required): The current status of the database connection, with valid values being available and unavailable.
- observedGeneration: Reflects the generation of the most recently observed RelationalDatabaseProvider object.

### RelationalDatabaseProvider Example

```yaml
apiVersion: v1alpha1
kind: RelationalDatabaseProvider
metadata:
  name: example-mysql-provider
spec:
  kind: mysql
  scope: development
  mysqlConnections:
    - name: primary-db
      hostname: primary-db.example.com
      port: 3306
      username: dbuser
      passwordSecretRef:
        name: mysql-pass
        key: password
      enabled: true
```

## DatabaseRequest CRD Documentation

The `DatabaseRequest` Custom Resource Definition (CRD) provides a mechanism for requesting database instances within a Kubernetes cluster. This resource allows lagoon to specify their requirements for a database, including the type, scope, and optional parameters such as seeding data, additional users, and database connection references. This documentation outlines the structure and functionality of the `DatabaseRequest` resource to facilitate its usage.

### DatabaseRequest Spec Fields

- name (required): Defines the intended service name for the database. This field is required and must be unique within the namespace.
- scope (required): Defines the intended use of the requested database. It helps in configuring the database appropriately for its intended environment. Valid options are production, development, and custom. The default value is development.
- type (required): Specifies the type of database requested. Supported types are mysql, mariadb, postgres, and mongodb.
- seed (optional): A reference to a local Kubernetes secret within the same namespace that contains data used for seeding the database. This field is optional and intended for initial database setup.
- additionalUsers (optional): Specifies the creation of additional database users.
- dropDatabaseOnDelete (optional): Determines whether the database should be automatically dropped (deleted) when the DatabaseRequest resource is deleted from Kubernetes. The default value is true.
- databaseConnectionReference (optional): A reference to an existing database connection. This field allows the request to specify an existing database connection to be used for the database operation.

- AdditionalUser Fields
    - type (required): The type of additional users to create. Can be either read-only or read-write, with read-only being the default.
    - name (required): The name of the additional service to add.

### DatabaseRequest Status Fields

- conditions: Provides detailed conditions of the DatabaseRequest such as readiness, errors, etc.
- DatabaseConnectionReference Fields
    - databaseObjectReference (required): A Kubernetes object reference to the database object.
    - name (required): The name of the database connection.
- observedGeneration: Reflects the generation of the most recently observed DatabaseRequest object.

### DatabaseRequest Example

```yaml
apiVersion: v1alpha1
kind: DatabaseRequest
metadata:
  name: example-database-request
spec:
  scope: development
  type: mysql
  additionalUsers:
    type: read-write
    count: 2
  dropDatabaseOnDelete: true
```

## Upgrade Process From Existing Operator

TBD
