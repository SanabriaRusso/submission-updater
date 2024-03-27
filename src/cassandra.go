package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sigv4-auth-cassandra-gocql-driver-plugin/sigv4"
	"github.com/gocql/gocql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// InitializeCassandraSession creates a new gocql session for Amazon Keyspaces using the provided configuration.
func InitializeCassandraSession(config *CassandraConfig) (*gocql.Session, error) {
	var cluster *gocql.ClusterConfig

	var endpoint string
	if config.CassandraHost == "" {
		if config.Region == "" {
			return nil, fmt.Errorf("AWS_REGION is required when CASSANDRA_HOST is not set")
		}
		endpoint = "cassandra." + config.Region + ".amazonaws.com"
	} else {
		endpoint = config.CassandraHost
	}

	cluster = gocql.NewCluster(endpoint)
	cluster.Keyspace = config.Keyspace

	var port int
	if config.CassandraPort != 0 {
		port = config.CassandraPort
	} else {
		port = 9142
	}
	cluster.Port = port

	if config.CassandraUsername != "" && config.CassandraPassword != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: config.CassandraUsername,
			Password: config.CassandraPassword}
	} else {
		var err error
		cluster.Authenticator, err = sigv4Authentication(config)
		if err != nil {
			return nil, fmt.Errorf("could not create SigV4 authenticator: %w", err)
		}
	}

	cluster.SslOpts = &gocql.SslOptions{
		CaPath: config.SSLCertificatePath,

		EnableHostVerification: false,
	}

	cluster.Consistency = gocql.LocalQuorum
	cluster.DisableInitialHostLookup = false
	cluster.RetryPolicy = &gocql.ExponentialBackoffRetryPolicy{NumRetries: 10, Min: 100 * time.Millisecond, Max: 10 * time.Second}

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("could not create Cassandra session: %w", err)
	}

	return session, nil
}

func sigv4Authentication(config *CassandraConfig) (sigv4.AwsAuthenticator, error) {
	auth := sigv4.NewAwsAuthenticator()
	if config.RoleSessionName != "" && config.RoleArn != "" && config.WebIdentityTokenFile != "" {
		// If role-related env variables are set, use temporary credentials
		tokenBytes, err := os.ReadFile(config.WebIdentityTokenFile)
		if err != nil {
			return auth, fmt.Errorf("error reading web identity token file: %w", err)
		}
		webIdentityToken := string(tokenBytes)

		awsSession, err := session.NewSession(&aws.Config{Region: aws.String(config.Region)})
		if err != nil {
			return auth, fmt.Errorf("error creating AWS session: %w", err)
		}

		stsSvc := sts.New(awsSession)
		creds, err := stsSvc.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          &config.RoleArn,
			RoleSessionName:  &config.RoleSessionName,
			WebIdentityToken: &webIdentityToken,
		})
		if err != nil {
			return auth, fmt.Errorf("unable to assume role: %w", err)
		}

		auth.AccessKeyId = *creds.Credentials.AccessKeyId
		auth.SecretAccessKey = *creds.Credentials.SecretAccessKey
		auth.SessionToken = *creds.Credentials.SessionToken
		auth.Region = config.Region
	} else {
		// Otherwise, use credentials from the config
		auth.AccessKeyId = config.AccessKeyId
		auth.SecretAccessKey = config.SecretAccessKey
		auth.Region = config.Region
	}
	return auth, nil
}

func (ctx *AppContext) selectRange(startTime, endTime time.Time) ([]Submission, error) {

	query := `SELECT submitted_at_date, shard, submitted_at, submitter, created_at, block_hash, 
			  raw_block, remote_addr, peer_id, snark_work, graphql_control_port, built_with_commit_sha, 
			  state_hash, parent, height, slot, validation_error, verified
              FROM submissions
              WHERE ` + calculateDateRange(startTime, endTime) +
		` AND ` + shardsToCql(calculateShardsInRange(startTime, endTime)) +
		` AND submitted_at >= ? AND submitted_at < ?`
	iter := ctx.CassandraSession.Query(query, startTime, endTime).Iter()

	var submissions []Submission
	for {
		// we need to scan into new submission object each time
		// otherwise we will end up with a slice of pointers to the same object
		var submission Submission
		if !iter.Scan(&submission.SubmittedAtDate, &submission.Shard, &submission.SubmittedAt, &submission.Submitter,
			&submission.CreatedAt, &submission.BlockHash, &submission.RawBlock, &submission.RemoteAddr, &submission.PeerID,
			&submission.SnarkWork, &submission.GraphqlControlPort, &submission.BuiltWithCommitSha, &submission.StateHash,
			&submission.Parent, &submission.Height, &submission.Slot, &submission.ValidationError, &submission.Verified) {
			break
		}
		submissions = append(submissions, submission)
	}
	if err := iter.Close(); err != nil {
		ctx.Log.Errorf("Error closing iterator: %s", err)
		return nil, err
	}

	return submissions, nil
}

func (ctx *AppContext) tryUpdateSubmissions(submissions []Submission) error {
	ctx.Log.Infof("Updating %d submissions", len(submissions))
	for _, sub := range submissions {
		// Update the submission
		// Note: raw_block and snark_work are reseted to nil since we don't want to keep them in the database
		query := `UPDATE submissions
                  SET state_hash = ?, parent = ?, height = ?, slot = ?, validation_error = ?, verified = ?, 
				  raw_block = ?, snark_work = ?
                  WHERE submitted_at_date = ? AND shard = ? AND submitted_at = ? AND submitter = ?`
		if err := ctx.CassandraSession.Query(query,
			sub.StateHash, sub.Parent, sub.Height, sub.Slot, sub.ValidationError, sub.Verified,
			nil, nil,
			sub.SubmittedAtDate, sub.Shard, sub.SubmittedAt, sub.Submitter).Exec(); err != nil {
			ctx.Log.Errorf("Failed to update submission: %v", err)
			return err
		}
	}
	ctx.Log.Infof("Submissions updated")

	return nil
}

func (ctx *AppContext) updateSubmissions(submissions []Submission) error {
	return ExponentialBackoff(func() error {
		if err := ctx.tryUpdateSubmissions(submissions); err != nil {
			ctx.Log.Errorf("Error updating submissions (trying again): %v", err)
			return err
		}
		return nil
	}, maxRetries, initialBackoff)
}

func calculateDateRange(startTime, endTime time.Time) string {
	var dateRange []string
	current := startTime

	// Ensure time components are stripped to compare dates only
	current = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
	endTimeDateOnly := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 0, 0, 0, 0, endTime.Location())

	for !current.After(endTimeDateOnly) {
		dateStr := current.Format("2006-01-02")
		dateRange = append(dateRange, dateStr)
		// Move to the next day
		current = current.AddDate(0, 0, 1)
	}

	inClause := strings.Join(dateRange, "','")
	inClause = fmt.Sprintf("submitted_at_date IN ('%s')", inClause)
	// fmt.Printf("calculateDateRange: %s\n", inClause)
	return inClause
}
