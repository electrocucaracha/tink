package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/packethost/pkg/log"
	"github.com/pkg/errors"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/tinkerbell/tink/db/migration"
	tb "github.com/tinkerbell/tink/protos/template"
	pb "github.com/tinkerbell/tink/protos/workflow"
)

// Database interface for tinkerbell database operations.
type Database interface {
	hardware
	template
	workflow
	WorkerWorkflow
}

type hardware interface {
	DeleteFromDB(ctx context.Context, id string) error
	InsertIntoDB(ctx context.Context, data string) error
	GetByMAC(ctx context.Context, mac string) (string, error)
	GetByIP(ctx context.Context, ip string) (string, error)
	GetByID(ctx context.Context, id string) (string, error)
	GetAll(fn func([]byte) error) error
}

type template interface {
	CreateTemplate(ctx context.Context, name string, data string, id uuid.UUID) error
	GetTemplate(ctx context.Context, fields map[string]string, deleted bool) (*tb.WorkflowTemplate, error)
	DeleteTemplate(ctx context.Context, name string) error
	ListTemplates(in string, fn func(id, n string, in, del *timestamp.Timestamp) error) error
	UpdateTemplate(ctx context.Context, name string, data string, id uuid.UUID) error
}

type workflow interface {
	CreateWorkflow(ctx context.Context, wf Workflow, data string, id uuid.UUID) error
	GetWorkflowMetadata(ctx context.Context, req *pb.GetWorkflowDataRequest) ([]byte, error)
	GetWorkflowDataVersion(ctx context.Context, workflowID string) (int32, error)
	GetWorkflow(ctx context.Context, id string) (Workflow, error)
	DeleteWorkflow(ctx context.Context, id string, state int32) error
	ListWorkflows(fn func(wf Workflow) error) error
	UpdateWorkflow(ctx context.Context, wf Workflow, state int32) error
	InsertIntoWorkflowEventTable(ctx context.Context, wfEvent *pb.WorkflowActionStatus, t time.Time) error
	ShowWorkflowEvents(wfID string, fn func(wfs *pb.WorkflowActionStatus) error) error
}

// WorkerWorkflow is an interface for methods invoked by APIs that the worker calls.
type WorkerWorkflow interface {
	InsertIntoWfDataTable(ctx context.Context, req *pb.UpdateWorkflowDataRequest) error
	GetfromWfDataTable(ctx context.Context, req *pb.GetWorkflowDataRequest) ([]byte, error)
	GetWorkflowsForWorker(ctx context.Context, id string) ([]string, error)
	UpdateWorkflowState(ctx context.Context, wfContext *pb.WorkflowContext) error
	GetWorkflowContexts(ctx context.Context, wfID string) (*pb.WorkflowContext, error)
	GetWorkflowActions(ctx context.Context, wfID string) (*pb.WorkflowActionList, error)
}

// TinkDB implements the Database interface.
type TinkDB struct {
	instance *sql.DB
	logger   log.Logger
}

// Connect returns a connection to postgres database.
func Connect(db *sql.DB, lg log.Logger) *TinkDB {
	return &TinkDB{instance: db, logger: lg}
}

func (d *TinkDB) Migrate() (int, error) {
	return migrate.Exec(d.instance, "postgres", migration.GetMigrations(), migrate.Up)
}

func (d *TinkDB) CheckRequiredMigrations() (int, error) {
	migrations := migration.GetMigrations().Migrations
	records, err := migrate.GetMigrationRecords(d.instance, "postgres")
	if err != nil {
		return 0, err
	}
	return len(migrations) - len(records), nil
}

// Error returns the underlying cause for error.
func Error(err error) *pq.Error {
	if pqErr, ok := errors.Cause(err).(*pq.Error); ok {
		return pqErr
	}
	return nil
}

func get(ctx context.Context, db *sql.DB, query string, args ...interface{}) (string, error) {
	row := db.QueryRowContext(ctx, query, args...)

	buf := []byte{}
	if err := row.Scan(&buf); err != nil {
		return "", errors.Wrap(err, "SELECT")
	}
	return string(buf), nil
}

// buildGetCondition builds a where condition string in the format "column_name = 'field_value' AND"
// takes in a map[string]string with keys being the column name and the values being the field values.
func buildGetCondition(fields map[string]string) (string, error) {
	for column, field := range fields {
		if field != "" {
			return fmt.Sprintf("%s = %s", pq.QuoteIdentifier(column), pq.QuoteLiteral(field)), nil
		}
	}
	return "", errors.New("one GetBy field must be set to build a get condition")
}
