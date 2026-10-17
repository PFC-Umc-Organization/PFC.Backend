package references

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type handlers struct{ svc *Service }

// The DOI goes in the query string, not the path: it contains "/".
func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}
	r.Handle("GET", "/referencias/busca", h.search)
	r.Handle("GET", "/referencias/doi", h.lookupDOI)
	r.Handle("GET", "/projetos/:projetoId/referencias", h.list)
	r.Handle("POST", "/projetos/:projetoId/referencias", h.add)
	r.Handle("DELETE", "/projetos/:projetoId/referencias/:referenciaId", h.remove)
}

func (h handlers) search(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	result, err := h.svc.Search(ctx, r.Query("q"), r.Query("pagina"))
	return httpapi.Reply(200, result, err)
}

func (h handlers) lookupDOI(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	w, err := h.svc.LookupDOI(ctx, r.Query("doi"))
	return httpapi.Reply(200, w, err)
}

func (h handlers) list(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	refs, err := h.svc.ReferencesOf(ctx, r.Actor, r.Param("projetoId"))
	return httpapi.Reply(200, refs, err)
}

func (h handlers) add(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ReferenceInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	ref, err := h.svc.Add(ctx, r.Actor, r.Param("projetoId"), in)
	return httpapi.Reply(201, ref, err)
}

func (h handlers) remove(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("referência removida", h.svc.Remove(ctx, r.Actor, r.Param("projetoId"), r.Param("referenciaId")))
}
