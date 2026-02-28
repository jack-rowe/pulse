package store

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jack-rowe/pulse/checker"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketResults = []byte("results") // bucket: endpoint_name -> sub-bucket of timestamp -> record
	bucketLatest  = []byte("latest")  // bucket: endpoint_name -> latest record
)

// BoltStore implements Store using BBolt (pure Go, single-file, zero CGO).
type BoltStore struct {
	db *bolt.DB
}

// NewBolt opens or creates a BBolt database at the given path.
func NewBolt(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt db: %w", err)
	}

	// Create top-level buckets
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketResults); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketLatest); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("creating buckets: %w", err)
	}

	return &BoltStore{db: db}, nil
}

// SaveResult persists a check record.
func (s *BoltStore) SaveResult(record CheckRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshaling record: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		// Save to history
		results := tx.Bucket(bucketResults)
		epBucket, err := results.CreateBucketIfNotExists([]byte(record.EndpointName))
		if err != nil {
			return fmt.Errorf("creating endpoint bucket: %w", err)
		}

		key := timeToKey(record.Timestamp)
		if err := epBucket.Put(key, data); err != nil {
			return err
		}

		// Update latest
		latest := tx.Bucket(bucketLatest)
		return latest.Put([]byte(record.EndpointName), data)
	})
}

// GetLatest returns the most recent result for each endpoint.
func (s *BoltStore) GetLatest() ([]CheckRecord, error) {
	var records []CheckRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketLatest)
		return b.ForEach(func(k, v []byte) error {
			var rec CheckRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			records = append(records, rec)
			return nil
		})
	})

	return records, err
}

// GetHistory returns check results for an endpoint, newest first.
func (s *BoltStore) GetHistory(endpointName string, limit int) ([]CheckRecord, error) {
	var records []CheckRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		results := tx.Bucket(bucketResults)
		epBucket := results.Bucket([]byte(endpointName))
		if epBucket == nil {
			return nil // no data yet
		}

		c := epBucket.Cursor()
		count := 0

		// Iterate in reverse (newest first)
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			if limit > 0 && count >= limit {
				break
			}
			var rec CheckRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			records = append(records, rec)
			count++
		}
		return nil
	})

	return records, err
}

// GetUptimeSummary computes uptime statistics over a given duration.
func (s *BoltStore) GetUptimeSummary(endpointName string, duration time.Duration) (*UptimeSummary, error) {
	summary := &UptimeSummary{EndpointName: endpointName}
	cutoff := time.Now().Add(-duration)

	err := s.db.View(func(tx *bolt.Tx) error {
		results := tx.Bucket(bucketResults)
		epBucket := results.Bucket([]byte(endpointName))
		if epBucket == nil {
			return nil
		}

		var totalLatency float64
		var minLat, maxLat float64
		c := epBucket.Cursor()
		cutoffKey := timeToKey(cutoff)

		for k, v := c.Seek(cutoffKey); k != nil; k, v = c.Next() {
			var rec CheckRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}

			summary.TotalChecks++
			totalLatency += rec.LatencyMs
			if rec.LatencyMs > maxLat {
				maxLat = rec.LatencyMs
			}
			if minLat == 0 || (rec.LatencyMs > 0 && rec.LatencyMs < minLat) {
				minLat = rec.LatencyMs
			}

			if rec.Status == checker.StatusDown {
				summary.TotalFailures++
				t := rec.Timestamp
				summary.LastDownAt = &t
			}
		}

		if summary.TotalChecks > 0 {
			summary.UptimePercent = float64(summary.TotalChecks-summary.TotalFailures) / float64(summary.TotalChecks) * 100
			summary.AvgLatencyMs = totalLatency / float64(summary.TotalChecks)
		}
		summary.MinLatencyMs = minLat
		summary.MaxLatencyMs = maxLat

		// Get current status from latest
		latest := tx.Bucket(bucketLatest)
		if v := latest.Get([]byte(endpointName)); v != nil {
			var rec CheckRecord
			if err := json.Unmarshal(v, &rec); err == nil {
				summary.CurrentStatus = rec.Status
				summary.LastChecked = rec.Timestamp
			}
		}

		return nil
	})

	return summary, err
}

// PurgeOlderThan removes records older than the given time. Returns count deleted.
func (s *BoltStore) PurgeOlderThan(before time.Time) (int, error) {
	deleted := 0

	err := s.db.Update(func(tx *bolt.Tx) error {
		results := tx.Bucket(bucketResults)
		beforeKey := timeToKey(before)

		return results.ForEachBucket(func(k []byte) error {
			epBucket := results.Bucket(k)
			if epBucket == nil {
				return nil
			}

			c := epBucket.Cursor()
			for key, _ := c.First(); key != nil; key, _ = c.Next() {
				// Keys are time-ordered; stop once we're past cutoff
				if bytes.Compare(key, beforeKey) >= 0 {
					break
				}
				if err := c.Delete(); err != nil {
					return err
				}
				deleted++
			}
			return nil
		})
	})

	return deleted, err
}

// Close closes the database.
func (s *BoltStore) Close() error {
	return s.db.Close()
}

// GetTimeline returns bucketed check results for timeline visualization.
func (s *BoltStore) GetTimeline(endpointName string, duration time.Duration, numBuckets int) ([]TimelineBucket, error) {
	now := time.Now()
	start := now.Add(-duration)
	bucketDuration := duration / time.Duration(numBuckets)

	// Initialize empty buckets
	buckets := make([]TimelineBucket, numBuckets)
	for i := range buckets {
		buckets[i] = TimelineBucket{
			Start:  start.Add(time.Duration(i) * bucketDuration),
			End:    start.Add(time.Duration(i+1) * bucketDuration),
			Status: "empty",
		}
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		results := tx.Bucket(bucketResults)
		epBucket := results.Bucket([]byte(endpointName))
		if epBucket == nil {
			return nil
		}

		c := epBucket.Cursor()
		startKey := timeToKey(start)

		// Accumulators per bucket
		type acc struct {
			totalLat float64
			maxLat   float64
			checks   int
			fails    int
		}
		accums := make([]acc, numBuckets)

		for k, v := c.Seek(startKey); k != nil; k, v = c.Next() {
			var rec CheckRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				continue
			}

			// Find which bucket this record belongs to
			offset := rec.Timestamp.Sub(start)
			idx := int(offset / bucketDuration)
			if idx < 0 {
				idx = 0
			}
			if idx >= numBuckets {
				idx = numBuckets - 1
			}

			accums[idx].checks++
			accums[idx].totalLat += rec.LatencyMs
			if rec.LatencyMs > accums[idx].maxLat {
				accums[idx].maxLat = rec.LatencyMs
			}
			if rec.Status == checker.StatusDown {
				accums[idx].fails++
			}
		}

		// Convert accumulators to buckets
		for i, a := range accums {
			buckets[i].TotalChecks = a.checks
			buckets[i].Failures = a.fails
			buckets[i].MaxLatencyMs = a.maxLat
			if a.checks > 0 {
				buckets[i].AvgLatencyMs = a.totalLat / float64(a.checks)
				if a.fails == a.checks {
					buckets[i].Status = "down"
				} else if a.fails > 0 {
					buckets[i].Status = "degraded"
				} else {
					buckets[i].Status = "up"
				}
			}
		}

		return nil
	})

	return buckets, err
}

// timeToKey converts a time to a sortable 8-byte key (unix nano, big-endian).
func timeToKey(t time.Time) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(t.UnixNano()))
	return b
}
