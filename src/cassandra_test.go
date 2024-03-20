package main

import (
	"testing"
	"time"
)

func TestCalculateDateRange(t *testing.T) {
	testCases := []struct {
		name      string
		startTime time.Time
		endTime   time.Time
		want      string
	}{
		{
			name:      "single day",
			startTime: time.Date(2023, 3, 15, 10, 0, 0, 0, time.UTC),
			endTime:   time.Date(2023, 3, 15, 15, 0, 0, 0, time.UTC),
			want:      "submitted_at_date IN ('2023-03-15')",
		},
		{
			name:      "multiple days",
			startTime: time.Date(2023, 3, 15, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2023, 3, 17, 0, 0, 0, 0, time.UTC),
			want:      "submitted_at_date IN ('2023-03-15','2023-03-16','2023-03-17')",
		},
		{
			name:      "one day in the future",
			startTime: time.Date(2023, 3, 17, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2023, 3, 18, 0, 0, 0, 0, time.UTC),
			want:      "submitted_at_date IN ('2023-03-17','2023-03-18')",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateDateRange(tc.startTime, tc.endTime)
			if got != tc.want {
				t.Errorf("calculateDateRange(%v, %v) = %v, want %v", tc.startTime, tc.endTime, got, tc.want)
			}
		})
	}
}
