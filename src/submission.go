package main

import (
	"encoding/json"
	"time"
)

// Submission represents a single submission to be verified
// It represents single row in the submissions table
type Submission struct {
	SubmittedAtDate    string    `json:"submitted_at_date"`
	Shard              int       `json:"shard"`
	SubmittedAt        time.Time `json:"submitted_at"`
	Submitter          string    `json:"submitter"`
	CreatedAt          time.Time `json:"created_at"`
	BlockHash          string    `json:"block_hash"`
	RawBlock           RawBlock  `json:"raw_block"`
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

type RawBlock []byte

// Custom JSON marshalling for RawBlock type
// This is necessary because the default marshalling of empty byte slice
// is "null" instead of ""
// and delegation-verify expects string for raw_block
// we need this such that delegation-verify doesn't stop processing in case we have empty raw_block
func (b RawBlock) MarshalJSON() ([]byte, error) {
	if len(b) == 0 {
		return []byte(`""`), nil
	}
	return json.Marshal([]byte(b))
}
