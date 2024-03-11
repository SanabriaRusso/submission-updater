package main

import (
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
	log := logging.Logger("Cassandra updater")
	startTime, endTime := parseArgs(log)

	appCfg := LoadEnv(log)
	session, err := InitializeCassandraSession(appCfg.CassandraConfig)
	if err != nil {
		log.Fatalf("Error initializing Keyspace session: %v", err)
	}
	defer session.Close()

	kc := CassandraContext{
		Session:  session,
		Keyspace: appCfg.CassandraConfig.Keyspace,
		Log:      log,
	}
	log.Infof("Cassandra session initialized")

	log.Infof("Selecting submissions in range: (%v, %v)", startTime.Format("2006-01-02 15:04:05.0-0700"), endTime.Format("2006-01-02 15:04:05.0-0700"))

	submissions, err := kc.selectRange(startTime, endTime)
	if err != nil {
		log.Fatalf("Error selecting range: %v", err)
	}
	log.Infof("Number of returned submissions: %v", len(submissions))

	// Update the submissions
	err = kc.updateSubmissions(submissions)
	if err != nil {
		log.Fatalf("Error updating submissions: %v", err)
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
