package tracelet

import (
	io4edgecore "github.com/ci4rail/tracelet_host/devsim/pkg/io4edge_core"
	"github.com/go-chi/chi/v5"
)

func RegistrarTracelet(tl *Tracelet) io4edgecore.RouteRegistrar {
	return func(api chi.Router) {
		api.Get("/pos/parameter", tl.posParams.ListParameterSetHandlerFunc())
		api.Get("/pos/parameter/{parameter}", tl.posParams.GetParameterHandlerFunc())
		api.Put("/pos/parameter/{parameter}", tl.posParams.PutParameterHandlerFunc())
		api.Get("/pos/parameterset", tl.posParams.GetParameterSetHandlerFunc())
		api.Put("/pos/parameterset", tl.posParams.PutParameterSetHandlerFunc())
	}
}
