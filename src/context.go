package main

import (
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gocql/gocql"
	logging "github.com/ipfs/go-log/v2"
)

// Context holds shared resources and configurations.
type Context struct {
	CassandraSession *gocql.Session
	S3Session        *s3.S3
	Log              *logging.ZapEventLogger
}

// NewContext creates a new context with the necessary components.
func NewContext(cassandraConfig *CassandraConfig, awsConfig *AwsConfig, log *logging.ZapEventLogger) (*Context, error) {
	cassandraSession, err := InitializeCassandraSession(cassandraConfig)
	if err != nil {
		return nil, err
	}

	s3Session, err := InitializeS3Session(awsConfig.Region)
	if err != nil {
		return nil, err
	}

	return &Context{
		CassandraSession: cassandraSession,
		Log:              log,
		S3Session:        s3Session,
	}, nil
}
