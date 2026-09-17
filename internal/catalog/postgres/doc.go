// Package postgres implements the catalog persistence port and the explicit
// PostgreSQL schema-administration boundary. Store has runtime data access;
// Migrator is a separate, privileged capability used only by deployment.
package postgres
