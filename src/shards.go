package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// calculateShard returns the shard number for a given submission time.
// 0-599 are the possible shard numbers, each representing a 144-second interval within 24h.
// shard = (3600 * hour + 60 * minute + second) // 144
func calculateShard(submittedAt time.Time) int {
	hour := submittedAt.Hour()
	minute := submittedAt.Minute()
	second := submittedAt.Second()
	return (3600*hour + 60*minute + second) / 144
}

func calculateShardsInRange(startTime, endTime time.Time) []int {
	shardsMap := make(map[int]bool)
	var uniqueShards []int

	current := startTime
	for current.Before(endTime) {
		shard := calculateShard(current)
		if !shardsMap[shard] {
			shardsMap[shard] = true
			uniqueShards = append(uniqueShards, shard)
		}
		// Move to the next second
		current = current.Add(time.Second)
	}

	// Check if endTime falls exactly on a new shard boundary and add it if necessary
	endShard := calculateShard(endTime)
	if _, exists := shardsMap[endShard]; !exists {
		// Add the shard if endTime is exactly on the boundary of a new shard
		// This check prevents adding an extra shard when endTime is within an existing shard range
		if endTime.Equal(startTime.Add(time.Duration(endShard*144) * time.Second)) {
			shardsMap[endShard] = true
			uniqueShards = append(uniqueShards, endShard)
		}
	}

	// Sort the unique shards for readability
	sort.Ints(uniqueShards)

	return uniqueShards
}

func shardsToCql(shards []int) string {
	// Convert the sorted slice of shards into a slice of strings
	shardStrs := make([]string, len(shards))
	for i, shard := range shards {
		shardStrs[i] = fmt.Sprintf("%d", shard)
	}

	// Format the shards into a CQL statement string
	shardsStr := strings.Join(shardStrs, ",")
	cqlStatement := fmt.Sprintf("shard in (%s)", shardsStr)
	return cqlStatement
}
