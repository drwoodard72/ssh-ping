package stats

import (
	"math"
	"sort"
	"time"
)

func Min(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	min := d[0]
	for _, v := range d[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func Max(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	max := d[0]
	for _, v := range d[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func Mean(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sum := int64(0)
	for _, v := range d {
		sum += v.Nanoseconds()
	}
	return time.Duration(sum / int64(len(d)))
}

func Median(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(d))
	copy(sorted, d)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func StdDev(d []time.Duration) time.Duration {
	if len(d) < 2 {
		return 0
	}
	mean := Mean(d)
	sumSq := 0.0
	for _, v := range d {
		diff := float64(v.Nanoseconds() - mean.Nanoseconds())
		sumSq += diff * diff
	}
	variance := sumSq / float64(len(d)-1)
	return time.Duration(math.Sqrt(variance))
}
