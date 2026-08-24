package provisioningrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	"github.com/gsoultan/anubis/internal/platform/mw"
	provisioningapp "github.com/gsoultan/anubis/internal/provisioning/app"
	provisioningdomain "github.com/gsoultan/anubis/internal/provisioning/domain"
	provisioningsvc "github.com/gsoultan/anubis/internal/provisioning/service"
)

// xlsxContentType is the media type Excel registers for .xlsx. It travels
// with the workbook so the console can name the download correctly without
// hard-coding a second copy of it.
const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ProvisioningHandler implements anubisv1connect.ProvisioningServiceHandler.
type ProvisioningHandler struct {
	svc provisioningsvc.ProvisioningService
	f   mw.Factory
}

func NewProvisioningHandler(svc provisioningsvc.ProvisioningService, f mw.Factory) *ProvisioningHandler {
	return &ProvisioningHandler{svc: svc, f: f}
}

var _ anubisv1connect.ProvisioningServiceHandler = (*ProvisioningHandler)(nil)

func (h *ProvisioningHandler) DownloadImportTemplate(ctx context.Context,
	_ *connect.Request[anubisv1.DownloadImportTemplateRequest],
) (*connect.Response[anubisv1.DownloadImportTemplateResponse], error) {
	out, err := h.f.Do(ctx, "admin.provisioning.template", func(ctx context.Context) (any, error) {
		return h.svc.ImportTemplate(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.DownloadImportTemplateResponse{
		Workbook:    out.([]byte),
		Filename:    provisioningapp.TemplateFilename,
		ContentType: xlsxContentType,
	}), nil
}

func (h *ProvisioningHandler) ImportWorkbook(ctx context.Context,
	req *connect.Request[anubisv1.ImportWorkbookRequest],
) (*connect.Response[anubisv1.ImportWorkbookResponse], error) {
	out, err := h.f.Do(ctx, "admin.provisioning.import", func(ctx context.Context) (any, error) {
		return h.svc.ImportWorkbook(ctx, provisioningapp.ImportInput{
			Data: req.Msg.Workbook,
			Dry:  req.Msg.Dry,
		})
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(reportProto(out.(*provisioningdomain.ImportReport))), nil
}

func reportProto(r *provisioningdomain.ImportReport) *anubisv1.ImportWorkbookResponse {
	resp := &anubisv1.ImportWorkbookResponse{
		Dry:                 r.Dry,
		Applied:             r.Applied,
		PeopleCreated:       int32(r.PeopleCreated),
		PeopleExisting:      int32(r.PeopleExisting),
		GrantsCreated:       int32(r.GrantsCreated),
		GrantsSkipped:       int32(r.GrantsSkipped),
		MembershipsAssigned: int32(r.MembershipsAssigned),
		MembershipsExisting: int32(r.MembershipsExisting),
		IssuesOmitted:       int32(r.IssuesOmitted),
	}
	resp.Issues = make([]*anubisv1.ImportIssue, 0, len(r.Issues))
	for _, i := range r.Issues {
		resp.Issues = append(resp.Issues, &anubisv1.ImportIssue{
			Sheet:   i.Sheet,
			Row:     int32(i.Row),
			Column:  i.Column,
			Message: i.Message,
		})
	}
	return resp
}
