// Package auth implements the session auth app module.
package auth

import (
	"time"

	"kumacore/app/middleware/auth"
	"kumacore/app/repositories/auth"
	"kumacore/app/services/auth"
	"kumacore/core/db/driver"
	"kumacore/core/module"
	"kumacore/core/render"
)

const moduleID = "auth"

// Module registers auth routes and session middleware.
type Module struct {
	handler *Handler
	service *authservice.Service
}

// New returns the auth scaffold module.
func New(
	renderer render.Renderer,
	databaseConnection driver.DBTX,
	sessionTTL time.Duration,
	isDevelopment bool,
) (*Module, error) {
	repository := authrepository.NewRepository(databaseConnection)
	service, err := authservice.NewService(repository, sessionTTL)
	if err != nil {
		return nil, err
	}

	return &Module{
		handler: NewHandler(renderer, service, isDevelopment),
		service: service,
	}, nil
}

// ID returns the module ID.
func (authModule *Module) ID() string {
	return moduleID
}

// Register contributes auth middleware and routes.
func (authModule *Module) Register(registrar module.Registrar) error {
	registrar.Middleware(authmiddleware.LoadAuth(authModule.service))
	registrar.Routes(Routes(authModule.handler))
	return nil
}
