package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gocql/gocql"
	logging "github.com/ipfs/go-log/v2"
)

// AppContext holds shared resources and configurations.
type AppContext struct {
	CassandraSession *gocql.Session
	PostgresSession  *sql.DB
	S3Session        *s3.Client
	AppConfig        AppConfig
	Log              *logging.ZapEventLogger
}

// NewAppContext creates a new context with the necessary components.
func NewAppContext(ctx context.Context, config AppConfig, log *logging.ZapEventLogger) (*AppContext, error) {
	var cassandraSession *gocql.Session
	var postgresSession *sql.DB
	var err error
	if config.SubmissionStorage == "CASSANDRA" {
		cassandraSession, err = InitializeCassandraSession(config.CassandraConfig)
		if err != nil {
			return nil, err
		}
	} else {
		postgresSession, err = InitializePostgresSession(config.PostgreSQLConfig)
		if err != nil {
			return nil, err
		}
	}

	s3Session, err := InitializeS3Session(ctx, config.AwsConfig.Region)
	if err != nil {
		return nil, err
	}

	return &AppContext{
		CassandraSession: cassandraSession,
		PostgresSession:  postgresSession,
		Log:              log,
		S3Session:        s3Session,
		AppConfig:        config,
	}, nil
}

func (ctx *AppContext) selectRange(startTime, endTime time.Time) ([]Submission, error) {
	if ctx.AppConfig.SubmissionStorage == "CASSANDRA" {
		return ctx.selectRangeCassandra(startTime, endTime)
	}
	return ctx.selectRangePostgres(startTime, endTime)
}

func (ctx *AppContext) updateSubmissions(submissions []Submission) error {
	if ctx.AppConfig.SubmissionStorage == "CASSANDRA" {
		return ctx.updateSubmissionsCassandra(submissions)
	}
	return ctx.updateSubmissionsPostgres(submissions)
}
