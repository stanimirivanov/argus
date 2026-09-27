// Package postgres implements Argus runtime persistence ports over the owned
// catalog schema and the explicit PostgreSQL schema-administration boundary.
// Store has bounded runtime data access; Migrator is a separate, privileged
// capability used only by deployment.
package postgres
