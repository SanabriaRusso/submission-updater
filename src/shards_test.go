package main

import (
	"reflect"
	"testing"
	"time"
)

func TestCalculateShard(t *testing.T) {
	tests := []struct {
		name        string
		submittedAt time.Time
		want        int
	}{
		{
			name:        "Midnight",
			submittedAt: time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC),
			want:        0,
		},
		{
			name:        "Half day",
			submittedAt: time.Date(2024, 3, 11, 12, 0, 0, 0, time.UTC),
			want:        300,
		},
		{
			name:        "End of day",
			submittedAt: time.Date(2024, 3, 11, 23, 59, 59, 0, time.UTC),
			want:        599,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateShard(tt.submittedAt); got != tt.want {
				t.Errorf("calculateShard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateShardsInRange(t *testing.T) {
	tests := []struct {
		name      string
		startTime time.Time
		endTime   time.Time
		want      []int
	}{
		{
			name:      "Single Shard",
			startTime: time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2024, 3, 11, 0, 2, 23, 0, time.UTC),
			want:      []int{0},
		},
		{
			name:      "Multiple Shards",
			startTime: time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2024, 3, 11, 0, 2, 24, 0, time.UTC),
			want:      []int{0, 1},
		},
		{
			name:      "Across Hour Boundary",
			startTime: time.Date(2024, 3, 11, 0, 58, 0, 0, time.UTC),
			endTime:   time.Date(2024, 3, 11, 1, 2, 0, 0, time.UTC),
			want:      []int{24, 25},
		},
		{
			name:      "Date Boundary",
			startTime: time.Date(2024, 3, 11, 23, 58, 0, 0, time.UTC),
			endTime:   time.Date(2024, 3, 12, 0, 2, 23, 0, time.UTC),
			want:      []int{0, 599},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			if got = calculateShardsInRange(tt.startTime, tt.endTime); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calculateShardsInRange() = %v, want %v", got, tt.want)
			}
		})
	}
}
