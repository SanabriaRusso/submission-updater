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
	var scanFailures int
	for rows.Next() {
		var submission Submission
		if err := rows.Scan(&submission.ID, &submission.SubmittedAtDate, &submission.SubmittedAt,
			&submission.Submitter, &submission.CreatedAt, &submission.BlockHash, &submission.RemoteAddr,
			&submission.PeerID, &submission.SnarkWork, &submission.GraphqlControlPort,
			&submission.BuiltWithCommitSha); err != nil {
			// One unreadable row must not cost us the rest of the window, but it is
			// a submission that will never be verified or updated, so say so loudly.
			ctx.Log.Errorf("Error scanning row, skipping it: %s", err)
			scanFailures++
			continue
		}
		submissions = append(submissions, submission)
	}

	if err := rows.Err(); err != nil {
		ctx.Log.Errorf("Error iterating rows: %s", err)
		return nil, err
	}

	if scanFailures > 0 {
		ctx.Log.Errorf("Skipped %d of %d rows that could not be scanned; those submissions will not be verified",
			scanFailures, scanFailures+len(submissions))
		// Nothing readable at all is a schema or driver problem, not a bad row.
		if len(submissions) == 0 {
			return nil, fmt.Errorf("all %d rows in range failed to scan", scanFailures)
		}
	}

	return submissions, nil
}

func (ctx *AppContext) updateSubmissionsPostgres(submissions []Submission) error {
	ctx.Log.Infof("Updating %d submissions", len(submissions))

	// We nullify snark_work to keep the space usage low
	const query = `UPDATE submissions
                  SET snark_work = NULL, state_hash = $1, parent = $2, height = $3, slot = $4, validation_error = $5, verified = $6
                  WHERE id = $7`

	var failed int
	var firstErr error
	for _, sub := range submissions {
		if _, err := ctx.PostgresSession.Exec(query,
			sub.StateHash, sub.Parent, sub.Height, sub.Slot, sub.ValidationError, sub.Verified,
			sub.ID); err != nil {
			// Keep going: a row that will not write must not hold back the rows
			// behind it, which belong to unrelated submitters.
			ctx.Log.Errorf("Failed to update submission id=%s submitter=%s: %v", sub.ID, sub.Submitter, err)
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
	}

	if failed > 0 {
		return fmt.Errorf("failed to update %d of %d submissions, first error: %w", failed, len(submissions), firstErr)
	}

	ctx.Log.Infof("Submissions updated")
	return nil
}
