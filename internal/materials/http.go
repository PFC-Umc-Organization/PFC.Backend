package materials

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type handlers struct{ svc *Service }

func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}

	r.Handle("GET", "/materiais", h.list)
	r.Handle("POST", "/materiais", httpapi.Staff(msgStaffOnly, h.create, httpapi.Audit("material.publicado")))
	r.Handle("DELETE", "/materiais/:materialId", httpapi.Staff(msgStaffOnly, h.remove, httpapi.Audit("material.removido")))
	r.Handle("POST", "/materiais/arquivos", httpapi.Staff(msgStaffOnly, h.requestUpload))
	r.Handle("GET", "/materiais/:materialId/arquivo", h.download)
}

func (h handlers) list(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	list, err := h.svc.List(ctx, r.Actor)
	return httpapi.Reply(200, list, err)
}

func (h handlers) create(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in MaterialInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	m, err := h.svc.Create(ctx, r.Actor, in)
	return httpapi.Reply(201, m, err)
}

func (h handlers) remove(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Material removido.", h.svc.Delete(ctx, r.Actor, r.Param("materialId")))
}

func (h handlers) requestUpload(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in UploadRequest
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	t, err := h.svc.RequestUpload(ctx, in)
	return httpapi.Reply(201, t, err)
}

func (h handlers) download(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	d, err := h.svc.Download(ctx, r.Actor, r.Param("materialId"))
	return httpapi.Reply(200, d, err)
}
