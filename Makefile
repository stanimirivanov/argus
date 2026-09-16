.PHONY: help fmt check test _not-configured

help:
	@echo Argus engineering-foundation command surface
	@echo   make fmt    Format project sources - not configured yet
	@echo   make check  Run static and policy checks - not configured yet
	@echo   make test   Run project tests - not configured yet
	@echo These targets fail closed until the M01 executable scaffold defines them.

fmt check test: _not-configured

_not-configured:
	@echo ERROR - executable verification is not configured yet. See M01 - Engineering foundation. 1>&2
	@echo Do not report this target as passed. Record it as not run/not configured. 1>&2
	@exit 2
