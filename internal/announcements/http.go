package announcements

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

// Rotas
type handlers struct{ svc *Service }

// Rotas
func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}

	r.Handle("GET", "/avisos", h.list)
	r.Handle("POST", "/avisos", httpapi.Staff(msgStaffOnly, h.create, httpapi.Audit("aviso.publicado")))
	r.Handle("DELETE", "/avisos/:avisoId", httpapi.Staff(msgStaffOnly, h.remove, httpapi.Audit("aviso.excluido")))
}

// Listagem
func (h handlers) list(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	list, err := h.svc.List(ctx, r.Actor, r.Query("turmaId"))
	return httpapi.Reply(200, list, err)
}

// Publicação
func (h handlers) create(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in AnnouncementInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	a, err := h.svc.Create(ctx, r.Actor, in)
	return httpapi.Reply(201, a, err)
}

// Exclusão
func (h handlers) remove(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Aviso excluído.", h.svc.Delete(ctx, r.Actor, r.Param("avisoId")))
}
