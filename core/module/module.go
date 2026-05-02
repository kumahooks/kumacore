// Package module defines explicit app-local module registration contracts.
package module

import (
	"context"
	"net/http"
	"time"

	"kumacore/core/httpx"
)

// Module is an app-local feature package registered by generated bootstrap code.
type Module interface {
	ID() string
	Register(registrar Registrar) error
}

// Registrar collects module contributions before core app startup wires them.
type Registrar interface {
	Routes(routeRegistrar RouteRegistrar)
	Middleware(middlewareRegistrar MiddlewareRegistrar)
	Jobs(jobRegistrar JobRegistrar)
}

// RouteRegistrar follows the meryl.moe module Routes pattern.
type RouteRegistrar = httpx.RouteRegistrar

// MiddlewareRegistrar is a standard HTTP middleware contribution.
type MiddlewareRegistrar = func(http.Handler) http.Handler

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

// JobRegistrars returns registered job contributions.
func (contributions Contributions) JobRegistrars() []JobRegistrar {
	return append([]JobRegistrar(nil), contributions.jobs...)
}
