package stats

import (
	"testing"
	"time"
)

func TestMin(t *testing.T) {
	tests := []struct {
		name     string
		durations []time.Duration
		expected time.Duration
	}{
		{
			name:     "single element",
			durations: []time.Duration{100 * time.Millisecond},
			expected: 100 * time.Millisecond,
		},
		{
			name:     "multiple elements",
			durations: []time.Duration{100 * time.Millisecond, 50 * time.Millisecond, 75 * time.Millisecond},
			expected: 50 * time.Millisecond,
		},
		{
			name:     "empty slice",
			durations: []time.Duration{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Min(tt.durations)
			if result != tt.expected {
				t.Errorf("Min(%v) = %v, expected %v", tt.durations, result, tt.expected)
			}
		})
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		name     string
		durations []time.Duration
		expected time.Duration
	}{
		{
			name:     "single element",
			durations: []time.Duration{100 * time.Millisecond},
			expected: 100 * time.Millisecond,
		},
		{
			name:     "multiple elements",
			durations: []time.Duration{100 * time.Millisecond, 50 * time.Millisecond, 75 * time.Millisecond},
			expected: 100 * time.Millisecond,
		},
		{
			name:     "empty slice",
			durations: []time.Duration{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Max(tt.durations)
			if result != tt.expected {
				t.Errorf("Max(%v) = %v, expected %v", tt.durations, result, tt.expected)
			}
		})
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		name      string
		durations []time.Duration
		expected  time.Duration
	}{
		{
			name:     "single element",
			durations: []time.Duration{100 * time.Millisecond},
			expected: 100 * time.Millisecond,
		},
		{
			name:     "multiple elements",
			durations: []time.Duration{100 * time.Millisecond, 50 * time.Millisecond, 75 * time.Millisecond},
			expected: 75 * time.Millisecond,
		},
		{
			name:     "empty slice",
			durations: []time.Duration{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Mean(tt.durations)
			if result != tt.expected {
				t.Errorf("Mean(%v) = %v, expected %v", tt.durations, result, tt.expected)
			}
		})
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		name      string
		durations []time.Duration
		expected  time.Duration
	}{
		{
			name:     "single element",
			durations: []time.Duration{100 * time.Millisecond},
			expected: 100 * time.Millisecond,
		},
		{
			name:     "odd number of elements",
			durations: []time.Duration{100 * time.Millisecond, 50 * time.Millisecond, 75 * time.Millisecond},
			expected: 75 * time.Millisecond,
		},
		{
			name:     "even number of elements",
			durations: []time.Duration{100 * time.Millisecond, 50 * time.Millisecond},
			expected: 75 * time.Millisecond,
		},
		{
			name:     "empty slice",
			durations: []time.Duration{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Median(tt.durations)
			if result != tt.expected {
				t.Errorf("Median(%v) = %v, expected %v", tt.durations, result, tt.expected)
			}
		})
	}
}

func TestStdDev(t *testing.T) {
	tests := []struct {
		name      string
		durations []time.Duration
		check     func(time.Duration) bool
	}{
		{
			name:     "single element (stddev 0)",
			durations: []time.Duration{100 * time.Millisecond},
			check: func(d time.Duration) bool {
				return d == 0
			},
		},
		{
			name:     "multiple elements",
			durations: []time.Duration{100 * time.Millisecond, 50 * time.Millisecond, 75 * time.Millisecond},
			check: func(d time.Duration) bool {
				// Rough check: stddev should be positive and less than max-min
				return d > 0 && d < 50*time.Millisecond
			},
		},
		{
			name:     "empty slice",
			durations: []time.Duration{},
			check: func(d time.Duration) bool {
				return d == 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StdDev(tt.durations)
			if !tt.check(result) {
				t.Errorf("StdDev(%v) = %v, failed validation", tt.durations, result)
			}
		})
	}
}
