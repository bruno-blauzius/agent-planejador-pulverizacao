package postgres

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func TestTranslateElevationErrorMarksAStatementTimeout(t *testing.T) {
	canceled := &pgconn.PgError{Code: sqlStateQueryCanceled, Message: "canceling statement due to statement timeout"}
	got := translateElevationError("elevation window", canceled)
	if !errors.Is(got, usecase.ErrElevationTimeout) || !errors.Is(got, error(canceled)) {
		t.Fatalf("a 57014 must be a timeout and keep its cause, got %v", got)
	}
	other := &pgconn.PgError{Code: "XX000", Message: "internal error"}
	if got := translateElevationError("elevation window", other); errors.Is(got, usecase.ErrElevationTimeout) {
		t.Fatalf("another SQLSTATE is not a timeout: %v", got)
	}
}

func TestReadFailureClassifiesADownloadCutInTheMiddle(t *testing.T) {
	cut := errors.New("unexpected EOF")
	if got := readFailure(cut); !errors.Is(got, usecase.ErrElevationUnavailable) || !errors.Is(got, cut) {
		t.Fatalf("a cut download is an unavailable block, got %v", got)
	}
	if got := readFailure(context.DeadlineExceeded); !errors.Is(got, context.DeadlineExceeded) || errors.Is(got, usecase.ErrElevationUnavailable) {
		t.Fatalf("a deadline stays a deadline, got %v", got)
	}
}

type brokenBody struct{ err error }

func (b brokenBody) Read([]byte) (int, error) { return 0, b.err }

func TestIngestTileReportsABrokenBodyAsUnavailable(t *testing.T) {
	// The body is read before the database is touched, so no pool is needed.
	repo := &Repository{}
	err := repo.IngestTile(context.Background(), "S21W047", domain.ElevationModel{}, brokenBody{io.ErrUnexpectedEOF})
	if !errors.Is(err, usecase.ErrElevationUnavailable) {
		t.Fatalf("error = %v, want ErrElevationUnavailable", err)
	}
	err = repo.IngestTile(context.Background(), "S21W047", domain.ElevationModel{}, brokenBody{context.DeadlineExceeded})
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, usecase.ErrElevationUnavailable) {
		t.Fatalf("error = %v, want a plain deadline", err)
	}
}

func TestIngestStatementTimeoutFollowsTheDeadline(t *testing.T) {
	if got := ingestStatementTimeout(context.Background()); got != "600000ms" {
		t.Errorf("without a deadline = %s, want the 10 minute ceiling", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	got := ingestStatementTimeout(ctx)
	if got == "600000ms" || !strings.HasSuffix(got, "ms") {
		t.Errorf("with a 90s deadline = %s, want the time left", got)
	}
	past, cancelPast := context.WithTimeout(context.Background(), -time.Second)
	defer cancelPast()
	if got := ingestStatementTimeout(past); got != "1000ms" {
		t.Errorf("past deadline = %s, want the 1s floor", got)
	}
}
