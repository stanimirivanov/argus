// Package postgres implements capability-scoped Argus persistence over one
// bounded PostgreSQL runtime pool and one separately opened migration pool.
// CatalogStore, ChangeStore, ExecutionStore, and AdaptationStore share pool
// lifecycle but expose only their own consumer ports. Migrator is a separate,
// privileged capability used only by deployment.
package postgres
