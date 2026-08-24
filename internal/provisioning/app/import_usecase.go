package provisioningapp

import (
	"context"

	provisioningdomain "github.com/gsoultan/anubis/internal/provisioning/domain"
)

// ImportUsecase is the people-and-access import surface: hand out the
// workbook to fill in, then take it back.
type ImportUsecase interface {
	// ImportTemplate is the empty workbook an operator fills in, as the
	// bytes of an .xlsx file.
	ImportTemplate(ctx context.Context) ([]byte, error)
	// ImportWorkbook reads an uploaded workbook and either applies it or,
	// for a dry run, reports exactly what applying it would do.
	ImportWorkbook(ctx context.Context, in ImportInput) (*provisioningdomain.ImportReport, error)
}
