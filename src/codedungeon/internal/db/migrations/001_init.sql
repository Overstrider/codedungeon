-- Migration 001: initial schema (schema_version=1).
-- Idempotent (all CREATE IF NOT EXISTS). Applied once on fresh DBs.
-- Historical only: schema.sql (embedded) is the authoritative current state;
-- migrations are consulted for version deltas.
-- Content same as schema.sql at v1 (pre-meta-OS era).
-- This file exists so `SELECT COUNT(*) FROM applied_migrations` works on v1 DBs.
SELECT 1;
