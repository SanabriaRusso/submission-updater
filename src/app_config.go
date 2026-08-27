package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

func LoadEnv(log logging.EventLogger) AppConfig {
	var config AppConfig

	submissionStorage := getSubmissionStorage()

	// delegation_verify bin path
	delegationVerifyBinPath := getEnvChecked("DELEGATION_VERIFY_BIN_PATH", log)
	noChecks := boolEnvChecked("NO_CHECKS", log)
	networkName := getEnvChecked("NETWORK_NAME", log)
	genesisLedgerFile := os.Getenv("GENESIS_LEDGER_FILE")

	// dual-verifier (hard fork cutover) configuration
	delegationVerifyBinPathPostFork := os.Getenv("DELEGATION_VERIFY_BIN_PATH_POST_FORK")
	genesisLedgerFilePostFork := os.Getenv("GENESIS_LEDGER_FILE_POST_FORK")
	forkCutoverTime, err := parseForkCutoverConfig(os.Getenv("FORK_CUTOVER_TIME"), delegationVerifyBinPathPostFork)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// AWS configurations
	bucketName := getEnvChecked("AWS_S3_BUCKET", log)
	awsRegion := os.Getenv("AWS_REGION")
	// if webIdentityTokenFile, roleSessionName and roleArn are set,
	// we are using AWS STS to assume a role and get temporary credentials
	// if they are not set, we are using AWS IAM user credentials
	webIdentityTokenFile := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")
	roleSessionName := os.Getenv("AWS_ROLE_SESSION_NAME")
	roleArn := os.Getenv("AWS_ROLE_ARN")
	// accessKeyId, secretAccessKey are not mandatory for production set up
	accessKeyId := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	var awsKeyspace, cassandraHost, cassandraUsername, cassandraPassword, sslCertificatePath string
	var cassandraPort, postgresPort int
	var postgresHost, postgresUser, postgresPassword, postgresDBName, postgresSSLMode string
	if submissionStorage == "CASSANDRA" {
		// AWSKeyspace/Cassandra configurations
		awsKeyspace = os.Getenv("AWS_KEYSPACE")
		sslCertificatePath = os.Getenv("SSL_CERTFILE")

		//service level connection
		cassandraHost = os.Getenv("CASSANDRA_HOST")
		cassandraPortStr := os.Getenv("CASSANDRA_PORT")
		var err error
		cassandraPort, err = strconv.Atoi(cassandraPortStr)
		if err != nil {
			cassandraPort = 9142
		}
		cassandraUsername = os.Getenv("CASSANDRA_USERNAME")
		cassandraPassword = os.Getenv("CASSANDRA_PASSWORD")
	} else {
		// PostgreSQL configurations
		postgresHost = os.Getenv("POSTGRES_HOST")
		postgresUser = os.Getenv("POSTGRES_USER")
		postgresPassword = os.Getenv("POSTGRES_PASSWORD")
		postgresDBName = os.Getenv("POSTGRES_DB")
		var err error
		postgresPort, err = strconv.Atoi(os.Getenv("POSTGRES_PORT"))
		if err != nil {
			log.Fatalf("Error parsing POSTGRES_PORT: %v", err)
		}
		postgresSSLMode = os.Getenv("POSTGRES_SSLMODE")
		if postgresSSLMode == "" {
			postgresSSLMode = "require"
		}

	}

	config.NetworkName = networkName
	config.DelegationVerifyBinPath = delegationVerifyBinPath
	config.NoChecks = noChecks
	config.GenesisLedgerFile = genesisLedgerFile
	config.ForkCutoverTime = forkCutoverTime
	config.DelegationVerifyBinPathPostFork = delegationVerifyBinPathPostFork
	config.GenesisLedgerFilePostFork = genesisLedgerFilePostFork
	config.SubmissionStorage = submissionStorage
	config.CassandraConfig = &CassandraConfig{
		Keyspace:             awsKeyspace,
		CassandraHost:        cassandraHost,
		CassandraPort:        cassandraPort,
		CassandraUsername:    cassandraUsername,
		CassandraPassword:    cassandraPassword,
		Region:               awsRegion,
		AccessKeyId:          accessKeyId,
		SecretAccessKey:      secretAccessKey,
		WebIdentityTokenFile: webIdentityTokenFile,
		RoleSessionName:      roleSessionName,
		RoleArn:              roleArn,
		SSLCertificatePath:   sslCertificatePath,
	}
	config.PostgreSQLConfig = &PostgreSQLConfig{
		Host:     postgresHost,
		Port:     postgresPort,
		User:     postgresUser,
		Password: postgresPassword,
		DBName:   postgresDBName,
		SSLMode:  postgresSSLMode,
	}
	config.AwsConfig = &AwsConfig{
		BucketName:      bucketName,
		Region:          awsRegion,
		AccessKeyId:     accessKeyId,
		SecretAccessKey: secretAccessKey,
	}

	return config
}

// parseForkCutoverConfig validates the dual-verifier (hard fork cutover) settings.
// An empty cutover means dual-verifier mode is disabled and nil is returned.
// When the cutover is set, it must be a valid RFC3339 timestamp and the
// post-fork delegation-verify binary path must be set as well.
func parseForkCutoverConfig(cutover, postForkBinPath string) (*time.Time, error) {
	if cutover == "" {
		return nil, nil
	}
	cutoverTime, err := time.Parse(time.RFC3339, cutover)
	if err != nil {
		return nil, fmt.Errorf("error parsing FORK_CUTOVER_TIME as RFC3339: %v", err)
	}
	if postForkBinPath == "" {
		return nil, fmt.Errorf("missing DELEGATION_VERIFY_BIN_PATH_POST_FORK environment variable (required when FORK_CUTOVER_TIME is set)")
	}
	return &cutoverTime, nil
}

var validStorageOptions = map[string]bool{
	"CASSANDRA": true,
	"POSTGRES":  true,
}

func getSubmissionStorage() string {
	storage := os.Getenv("SUBMISSION_STORAGE")
	if storage == "" {
		storage = "POSTGRES" // Set default to "POSTGRES"
	}
	storage = strings.ToUpper(storage)

	// Validate the storage option
	if _, valid := validStorageOptions[storage]; !valid {
		log.Fatalf("Invalid storage option: %s. Valid options are %v", storage, validStorageOptions)
	}
	return storage
}

func getEnvChecked(variable string, log logging.EventLogger) string {
	value := os.Getenv(variable)
	if value == "" {
		log.Fatalf("missing %s environment variable", variable)
	}
	return value
}

func boolEnvChecked(variable string, log logging.EventLogger) bool {
	value := os.Getenv(variable)
	switch value {
	case "1":
		return true
	case "0":
		return false
	case "":
		return false
	default:
		log.Fatalf("%s, if set, should be either 0 or 1!", variable)
		return false
	}
}

type AwsConfig struct {
	BucketName      string `json:"bucket_name"`
	Region          string `json:"region"`
	AccessKeyId     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

type CassandraConfig struct {
	Keyspace             string `json:"keyspace"`
	CassandraHost        string `json:"cassandra_host"`
	CassandraPort        int    `json:"cassandra_port"`
	CassandraUsername    string `json:"cassandra_username,omitempty"`
	CassandraPassword    string `json:"cassandra_password,omitempty"`
	Region               string `json:"region,omitempty"`
	AccessKeyId          string `json:"access_key_id,omitempty"`
	SecretAccessKey      string `json:"secret_access_key,omitempty"`
	WebIdentityTokenFile string `json:"web_identity_token_file,omitempty"`
	RoleSessionName      string `json:"role_session_name,omitempty"`
	RoleArn              string `json:"role_arn,omitempty"`
	SSLCertificatePath   string `json:"ssl_certificate_path"`
}

type PostgreSQLConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
	SSLMode  string `json:"sslmode"`
}

type AppConfig struct {
	NetworkName             string `json:"network_name"`
	DelegationVerifyBinPath string `json:"delegation_verify_bin_path"`
	NoChecks                bool   `json:"no_checks"`
	GenesisLedgerFile       string `json:"genesis_ledger_file"`
	// ForkCutoverTime enables dual-verifier mode when set: submissions with
	// submitted_at >= ForkCutoverTime are verified with the post-fork binary,
	// all others with the (pre-fork) DelegationVerifyBinPath.
	ForkCutoverTime                 *time.Time        `json:"fork_cutover_time,omitempty"`
	DelegationVerifyBinPathPostFork string            `json:"delegation_verify_bin_path_post_fork,omitempty"`
	GenesisLedgerFilePostFork       string            `json:"genesis_ledger_file_post_fork,omitempty"`
	SubmissionStorage               string            `json:"submission_storage"`
	AwsConfig                       *AwsConfig        `json:"aws"`
	CassandraConfig                 *CassandraConfig  `json:"cassandra_config,omitempty"`
	PostgreSQLConfig                *PostgreSQLConfig `json:"postgres_config,omitempty"`
}
