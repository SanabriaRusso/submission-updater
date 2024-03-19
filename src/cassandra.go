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
	logging "github.com/ipfs/go-log/v2"
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

type CassandraContext struct {
	Session  *gocql.Session
	Keyspace string
	Log      *logging.ZapEventLogger
}

type Submission struct {
	SubmittedAtDate    string    `json:"submitted_at_date"`
	Shard              int       `json:"shard"`
	SubmittedAt        time.Time `json:"submitted_at"`
	Submitter          string    `json:"submitter"`
	CreatedAt          time.Time `json:"created_at"`
	BlockHash          string    `json:"block_hash"`
	RawBlock           []byte    `json:"raw_block"`
	RemoteAddr         string    `json:"remote_addr"`
	PeerID             string    `json:"peer_id"`
	SnarkWork          []byte    `json:"snark_work"`
	GraphqlControlPort int       `json:"graphql_control_port"`
	BuiltWithCommitSha string    `json:"built_with_commit_sha"`
	StateHash          string    `json:"state_hash"`
	Parent             string    `json:"parent"`
	Height             int       `json:"height"`
	Slot               int       `json:"slot"`
	ValidationError    string    `json:"validation_error"`
	Verified           bool      `json:"verified"`
}

func (kc *CassandraContext) selectRange(startTime, endTime time.Time) ([]Submission, error) {

	query := `SELECT submitted_at_date, shard, submitted_at, submitter, created_at, block_hash, raw_block, remote_addr, peer_id, snark_work, graphql_control_port, built_with_commit_sha, state_hash, parent, height, slot, validation_error, verified
              FROM submissions
              WHERE ` + calculateDateRange(startTime, endTime) +
		` AND ` + shardsToCql(calculateShardsInRange(startTime, endTime)) +
		` AND submitted_at >= ? AND submitted_at < ?`
	iter := kc.Session.Query(query, startTime, endTime).Iter()

	var submissions []Submission
	var submission Submission
	for iter.Scan(&submission.SubmittedAtDate, &submission.Shard, &submission.SubmittedAt, &submission.Submitter, &submission.CreatedAt, &submission.BlockHash, &submission.RawBlock, &submission.RemoteAddr, &submission.PeerID, &submission.SnarkWork, &submission.GraphqlControlPort, &submission.BuiltWithCommitSha, &submission.StateHash, &submission.Parent, &submission.Height, &submission.Slot, &submission.ValidationError, &submission.Verified) {
		submissions = append(submissions, submission)
	}
	if err := iter.Close(); err != nil {
		kc.Log.Errorf("Error closing iterator: %s", err)
		return nil, err
	}

	return submissions, nil
}

func (kc *CassandraContext) tryUpdateSubmissions(submissions []Submission) error {
	kc.Log.Infof("Updating %d submissions", len(submissions))
	for _, sub := range submissions {
		query := `UPDATE submissions
                  SET state_hash = ?, parent = ?, height = ?, slot = ?, validation_error = ?, verified = ?
                  WHERE submitted_at_date = ? AND shard = ? AND submitted_at = ? AND submitter = ?`
		if err := kc.Session.Query(query,
			sub.StateHash, sub.Parent, sub.Height, sub.Slot, sub.ValidationError, sub.Verified,
			sub.SubmittedAtDate, sub.Shard, sub.SubmittedAt, sub.Submitter).Exec(); err != nil {
			kc.Log.Errorf("Failed to update submission: %v", err)
			return err
		}
	}
	kc.Log.Infof("Submissions updated")

	return nil
}

func (kc *CassandraContext) updateSubmissions(submissions []Submission) error {
	return ExponentialBackoff(func() error {
		if err := kc.tryUpdateSubmissions(submissions); err != nil {
			kc.Log.Errorf("Error updating submissions (trying again): %v", err)
			return err
		}
		return nil
	}, maxRetries, initialBackoff)
}

// func (kc *CassandraContext) updateSubmissionsBatch(submissions []Submission) error {
// 	batch := kc.Session.NewBatch(gocql.LoggedBatch) // Use gocql.UnloggedBatch for unlogged batches

// 	// Define your dummy values here
// 	dummyStateHash := "dummy_state_hash"
// 	dummyParent := "dummy_parent"
// 	dummyHeight := 123
// 	dummySlot := 456
// 	dummyValidationError := "dummy_error"
// 	dummyVerified := true

// 	kc.Log.Infof("Updating %d submissions in batch", len(submissions))
// 	for _, sub := range submissions {
// 		batch.Query(`UPDATE submissions
//             SET state_hash = ?, parent = ?, height = ?, slot = ?, validation_error = ?, verified = ?
//             WHERE submitted_at_date = ? AND shard = ? AND submitted_at = ? AND submitter = ?`,
// 			dummyStateHash, dummyParent, dummyHeight, dummySlot, dummyValidationError, dummyVerified,
// 			sub.SubmittedAtDate, sub.Shard, sub.SubmittedAt, sub.Submitter)
// 	}

// 	// Execute the batch
// 	if err := kc.Session.ExecuteBatch(batch); err != nil {
// 		kc.Log.Errorf("Failed to execute batch update: %v", err)
// 		return err
// 	}
// 	kc.Log.Infof("Submissions updated in batch")

// 	return nil
// }

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
