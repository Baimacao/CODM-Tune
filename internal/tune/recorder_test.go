package tune

import (
	"testing"
	"time"
)

func TestRecordSmoothnessAndShortSession(t *testing.T) {
	start := time.Now()
	good := summarizeRecord(start, start.Add(10*time.Second), []float64{120, 121, 119, 122, 120, 118, 121, 120, 119, 121})
	if good.Average < 119 || good.Low < 110 || good.Score < 85 || good.Verdict == "样本不足" {
		t.Fatalf("smooth session misclassified: %+v", good)
	}
	bad := summarizeRecord(start, start.Add(10*time.Second), []float64{120, 20, 118, 30, 119, 25, 117, 28, 120, 22})
	if bad.Score >= good.Score || bad.Drops <= 0 || bad.Verdict == "极流畅" {
		t.Fatalf("rough session misclassified: good=%+v bad=%+v", good, bad)
	}
	short := summarizeRecord(start, start.Add(time.Second), []float64{120})
	if short.Verdict != "样本不足" {
		t.Fatalf("short session should be rejected: %+v", short)
	}
}
