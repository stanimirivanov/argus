package commandline

// LocalDatabaseURL is the loopback-only, disposable development database
// configured by compose.local.yaml. Commands use it only with explicit --local.
const LocalDatabaseURL = "postgres://argus_dev:argus_dev@127.0.0.1:55432/argus?sslmode=disable"
