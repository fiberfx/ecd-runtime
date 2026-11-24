// Package ecrtest gives the tests of a host the reference engine of Ecd Runtime, from the module of the facade package
// ecr, as package httptest gives the tests of an HTTP server a recorder.
//
// The model is the reference engine of tests: a map-based implementation of the Engine seam that states its behaviour
// in its plainest form, against which every other engine is compared. A host never runs it: Open of package ecr
// builds Jacquard, or the engine of its Options. ecrtest is the one package of the module that imports the model.
package ecrtest

import (
	"github.com/fiberfx/ecd-runtime/go/ecr"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
)

// NewModel returns a new model for the catalog, the reference engine of tests. It refuses what model.New refuses,
// with the same error and no engine. The model answers Name with "model".
func NewModel(cat *ecr.Catalog) (ecr.Engine, error) { return model.New(cat) }
