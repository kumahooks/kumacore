// Package module defines explicit app-local module registration contracts.
package module

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"kumacore/core/httpx"
)

// Module is an app-local feature package registered by generated bootstrap code.
type Module interface {
	ID() string
	Manifest() Manifest
	Register(registrar Registrar) error
}

// Manifest describes module identity and dependency requirements.
type Manifest struct {
	Name            string
	Version         string
	DependsOn       []string
	ConfigKeyPrefix string
}

// Registrar collects module contributions before core app startup wires them.
type Registrar interface {
	Routes(routeRegistrar RouteRegistrar)
	Middleware(middlewareRegistrar MiddlewareRegistrar)
	Migrations(migrationRegistrar MigrationRegistrar)
	Jobs(jobRegistrar JobRegistrar)
}

// RouteRegistrar follows the meryl.moe module Routes pattern.
type RouteRegistrar = httpx.RouteRegistrar

// MiddlewareRegistrar is a standard HTTP middleware contribution.
type MiddlewareRegistrar = func(http.Handler) http.Handler

// MigrationRegistrar describes a module migration source for one backend.
type MigrationRegistrar struct {
	ModuleID   string
	Backend    string
	FileSystem fs.FS
	Directory  string
}

// JobRegistrar describes a worker job contribution.
type JobRegistrar struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context, payload any) error
}

// Contributions stores module registration output.
type Contributions struct {
	routes     []RouteRegistrar
	middleware []MiddlewareRegistrar
	migrations []MigrationRegistrar
	jobs       []JobRegistrar
}

// Routes appends a route registrar.
func (contributions *Contributions) Routes(routeRegistrar RouteRegistrar) {
	contributions.routes = append(contributions.routes, routeRegistrar)
}

// Middleware appends a middleware registrar.
func (contributions *Contributions) Middleware(middlewareRegistrar MiddlewareRegistrar) {
	contributions.middleware = append(contributions.middleware, middlewareRegistrar)
}

// Migrations appends a migration registrar.
func (contributions *Contributions) Migrations(migrationRegistrar MigrationRegistrar) {
	contributions.migrations = append(contributions.migrations, migrationRegistrar)
}

// Jobs appends a job registrar.
func (contributions *Contributions) Jobs(jobRegistrar JobRegistrar) {
	contributions.jobs = append(contributions.jobs, jobRegistrar)
}

// RouteRegistrars returns registered route contributions.
func (contributions Contributions) RouteRegistrars() []RouteRegistrar {
	return append([]RouteRegistrar(nil), contributions.routes...)
}

// MiddlewareRegistrars returns registered middleware contributions.
func (contributions Contributions) MiddlewareRegistrars() []MiddlewareRegistrar {
	return append([]MiddlewareRegistrar(nil), contributions.middleware...)
}

// MigrationRegistrars returns registered migration contributions.
func (contributions Contributions) MigrationRegistrars() []MigrationRegistrar {
	return append([]MigrationRegistrar(nil), contributions.migrations...)
}

// JobRegistrars returns registered job contributions.
func (contributions Contributions) JobRegistrars() []JobRegistrar {
	return append([]JobRegistrar(nil), contributions.jobs...)
}
