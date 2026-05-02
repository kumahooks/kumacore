// Package home implements the home page scaffold module.
package home

import (
	"kumacore/core/module"
	"kumacore/core/render"
)

const moduleID = "home"

// Module registers the home page routes.
type Module struct {
	handler *Handler
}

// New returns the home scaffold module.
func New(renderer render.Renderer, applicationName string) *Module {
	return &Module{
		handler: NewHandler(renderer, applicationName),
	}
}

// ID returns the module ID.
func (homeModule *Module) ID() string {
	return moduleID
}

// Register contributes home routes.
func (homeModule *Module) Register(registrar module.Registrar) error {
	registrar.Routes(Routes(homeModule.handler))
	return nil
}
