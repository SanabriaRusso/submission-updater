package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

func main() {
	logging.SetupLogging(logging.Config{
		Format: logging.JSONOutput,
		Stderr: false,
		Stdout: true,
		Level:  logging.LevelDebug,
		File:   "",
	})
	log := logging.Logger("Submission Updater")
	startTime, endTime := parseArgs(log)

	appCfg := LoadEnv(log)
	ctx := context.Background()

	log.Info("Submission Updater started...")
	log.Info("Using SUBMISSION_STORAGE: ", appCfg.SubmissionStorage)
	log.Infof("Using DELEGATION_VERIFY_BIN_PATH: %v", appCfg.DelegationVerifyBinPath)
	session, err := InitializeCassandraSession(appCfg.CassandraConfig)
	if err != nil {
		log.Fatalf("Error initializing Keyspace session: %v", err)
	}
	defer session.Close()

	appCtx, err := NewAppContext(ctx, appCfg, log)
	if err != nil {
		log.Fatalf("Error creating context: %v", err)
	}

	log.Infof("S3 session initialized")
	log.Infof("Selecting submissions in range: (%v, %v)", startTime.Format("2006-01-02 15:04:05.0-0700"), endTime.Format("2006-01-02 15:04:05.0-0700"))

	submissions, err := appCtx.selectRange(startTime, endTime)
	if err != nil {
		log.Fatalf("Error selecting range: %v", err)
	}
	numberOfReturnedSubmissions := len(submissions)
	log.Infof("Number of returned submissions: %v", numberOfReturnedSubmissions)

	if numberOfReturnedSubmissions == 0 {
		log.Info("No submissions to verify")
		os.Exit(0)
	} else {
		log.Info("Adding potentialy missing blocks from S3...")
		submissions = appCtx.addMissingBlocksFromS3(ctx, submissions, appCfg)

		log.Info("Running delegation verification...")
		submissionsJSON, err := json.Marshal(submissions)
		if err != nil {
			log.Fatalf("Error marshaling submissions to JSON: %v", err)
		}

		// Run the delegation verification binary
		verifiedSubmissions, err := appCtx.runDelegationVerifyCommand(appCfg.DelegationVerifyBinPath, string(submissionsJSON))
		if err != nil {
			log.Fatalf("Error running command: %v", err)
		}

		// Update the submissions
		err = appCtx.updateSubmissions(verifiedSubmissions)
		if err != nil {
			log.Fatalf("Error updating submissions: %v", err)
		}
	}
}

func parseArgs(log logging.EventLogger) (startTime time.Time, endTime time.Time) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: <program> <start date> <end date>")
		os.Exit(1)
	}

	startDate := os.Args[1]
	endDate := os.Args[2]

	var err error
	startTime, err = time.Parse("2006-01-02 15:04:05.0-0700", startDate)
	if err != nil {
		log.Fatalf("Error parsing start date:", err)
	}

	endTime, err = time.Parse("2006-01-02 15:04:05.0-0700", endDate)
	if err != nil {
		log.Fatalf("Error parsing end date:", err)
	}

	return startTime, endTime
}
