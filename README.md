# DBaaS Controller

## Overview

The dbaas-controller is designed to be used by Lagoon, specifically focusing provisioning database access for Lagoon workloads. The dbaas-controller aims to facilitate easier updates, migrations, and overall management of database resources in Lagoon environments.

It allows for provisiong and deprovisioning of shared MySQL/MariaDB, PostgreSQL, and MongoDB databases.

## Installation

See [lagoon-charts](https://github.com/uselagoon/lagoon-charts)

## Previously Used Operator

The existing [DBaaS operator](https://github.com/amazeeio/dbaas-operator), while reliable, has several issues that complicate the upgrade and migration processes. The reliance on CRDs for storing critical information makes it challenging to update consumers without risking deprovisioning and data loss. Additionally, changing an environment type after creation can cause errors with deprovisioning, leading to a suboptimal experience.

The current DBaaS operator uses a Provider CRD to contain provisioning details and allows providers to be pooled by sharing the same type. It interacts with Lagoon environments by creating a Consumer CRD with the requested provider type, which then triggers the provisioning of database resources.
Proposed Solution

## Differences from the Existing Operator

The new dbaas-controller introduces significant changes to the provisioning and management of database resources. It leverages a DatabaseRequest resource for provisioning, which tracks the status of the request. The controller will also support automatic updates to hostnames, reduce reliance on CRDs for data storage, and utilize Kubernetes native items like kind: Secrets and kind: Services.

Key Features:

- Provisioning Resource: Uses DatabaseRequest instead of direct HTTP requests for provisioning databases.
- Automatic Updates: Enables automatic updates to consumer services if a provider's details change.
- Utilizes native Kubernetes secret resources for storing database credentials and connection details.
- Pooling and Disabling Providers: Supports adding new providers to a pool and marking providers as disabled or unable to deprovision.
- Migration Support: Offers mechanisms to migrate consumers between providers seamlessly.

## Custom Resource Definitions

To interact with the dbaas-controller, the following CRDs are introduced:

- DatabaseProvider
- DatabaseRequest
- DatabaseMigration

## Upgrade Process From Existing Operator

TBD
