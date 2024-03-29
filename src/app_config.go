package main

import (
	"os"
	"strconv"

	logging "github.com/ipfs/go-log/v2"
)

func LoadEnv(log logging.EventLogger) AppConfig {
	var config AppConfig

	// delegation_verify bin path
	delegationVerifyBinPath := getEnvChecked("DELEGATION_VERIFY_BIN_PATH", log)
	noChecks := boolEnvChecked("NO_CHECKS", log)
	networkName := getEnvChecked("NETWORK_NAME", log)

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

	// AWSKeyspace/Cassandra configurations
	awsKeyspace := getEnvChecked("AWS_KEYSPACE", log)
	sslCertificatePath := getEnvChecked("SSL_CERTFILE", log)

	//service level connection
	cassandraHost := os.Getenv("CASSANDRA_HOST")
	cassandraPortStr := os.Getenv("CASSANDRA_PORT")
	cassandraPort, err := strconv.Atoi(cassandraPortStr)
	if err != nil {
		cassandraPort = 9142
	}
	cassandraUsername := os.Getenv("CASSANDRA_USERNAME")
	cassandraPassword := os.Getenv("CASSANDRA_PASSWORD")

	config.NetworkName = networkName
	config.DelegationVerifyBinPath = delegationVerifyBinPath
	config.NoChecks = noChecks
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
	config.AwsConfig = &AwsConfig{
		BucketName:      bucketName,
		Region:          awsRegion,
		AccessKeyId:     accessKeyId,
		SecretAccessKey: secretAccessKey,
	}

	return config
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

type AppConfig struct {
	NetworkName             string           `json:"network_name"`
	DelegationVerifyBinPath string           `json:"delegation_verify_bin_path"`
	NoChecks                bool             `json:"no_checks"`
	AwsConfig               *AwsConfig       `json:"aws"`
	CassandraConfig         *CassandraConfig `json:"cassandra_config"`
}
