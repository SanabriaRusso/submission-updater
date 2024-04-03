package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gocql/gocql"
	logging "github.com/ipfs/go-log/v2"
)

// AppContext holds shared resources and configurations.
type AppContext struct {
	CassandraSession *gocql.Session
	S3Session        *s3.Client
	AppConfig        AppConfig
	Log              *logging.ZapEventLogger
}

// NewAppContext creates a new context with the necessary components.
func NewAppContext(ctx context.Context, config AppConfig, log *logging.ZapEventLogger) (*AppContext, error) {
	cassandraSession, err := InitializeCassandraSession(config.CassandraConfig)
	if err != nil {
		return nil, err
	}

	s3Session, err := InitializeS3Session(ctx, config.AwsConfig.Region)
	if err != nil {
		return nil, err
	}

	return &AppContext{
		CassandraSession: cassandraSession,
		Log:              log,
		S3Session:        s3Session,
		AppConfig:        config,
	}, nil
}
