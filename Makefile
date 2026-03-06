include build/Makefile

.PHONY: run-api run-worker run-asynqmon run-stack migration-up migration-down migration-status migration-reset

migration-up: migrate-up
migration-down: migrate-down
migration-status: migrate-status
migration-reset: migrate-reset
