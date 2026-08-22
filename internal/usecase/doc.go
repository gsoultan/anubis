// Package usecase holds the application's business flows, one usecase per
// operation for the security-critical paths. Usecases depend on
// internal/repository interfaces and internal/crypto only — never on proto,
// pgx, connect or go-kit. Services (internal/service) compose usecases;
// endpoints wrap services; transports speak wire formats.
package usecase
