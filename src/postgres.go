package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

func InitializePostgresSession(cfg *PostgreSQLConfig) (*sql.DB, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func (ctx *AppContext) selectRangePostgres(startTime, endTime time.Time) ([]Submission, error) {

	query := `SELECT id, submitted_at_date, submitted_at, submitter, created_at, block_hash,
              remote_addr, peer_id, snark_work, graphql_control_port, built_with_commit_sha
              FROM submissions
              WHERE submitted_at >= $1 AND submitted_at < $2`

	rows, err := ctx.PostgresSession.Query(query, startTime, endTime)
	if err != nil {
		ctx.Log.Errorf("Error executing query: %s", err)
		return nil, err
	}
	defer rows.Close()

	var submissions []Submission
	for rows.Next() {
		var submission Submission
		if err := rows.Scan(&submission.ID, &submission.SubmittedAtDate, &submission.SubmittedAt,
			&submission.Submitter, &submission.CreatedAt, &submission.BlockHash, &submission.RemoteAddr,
			&submission.PeerID, &submission.SnarkWork, &submission.GraphqlControlPort,
			&submission.BuiltWithCommitSha); err != nil {
			ctx.Log.Errorf("Error scanning row: %s", err)
			continue
		}
		submissions = append(submissions, submission)
	}

	if err := rows.Err(); err != nil {
		ctx.Log.Errorf("Error iterating rows: %s", err)
		return nil, err
	}

	return submissions, nil
}

func (ctx *AppContext) updateSubmissionsPostgres(submissions []Submission) error {
	ctx.Log.Infof("Updating %d submissions", len(submissions))

	for _, sub := range submissions {
		query := `UPDATE submissions
                  SET state_hash = $1, parent = $2, height = $3, slot = $4, validation_error = $5, verified = $6
                  WHERE id = $7`
		if _, err := ctx.PostgresSession.Exec(query,
			sub.StateHash, sub.Parent, sub.Height, sub.Slot, sub.ValidationError, sub.Verified,
			sub.ID); err != nil {
			ctx.Log.Errorf("Failed to update submission: %v", err)
			return err
		}
	}

	ctx.Log.Infof("Submissions updated")
	return nil
}
