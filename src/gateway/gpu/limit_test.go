package gpu

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const smiOutLimit = `0, 00000000:03:00.0, 71.40, 7421, 87, 61, 100.00, 250.00
1, 00000000:04:00.0, 101.10, 7380, 85, 63, 100.00, 250.00
2, 00000000:05:00.0, [N/A], 7402, 0, 40, [N/A], [N/A]
`

func TestParseCSVWithPowerLimit(t *testing.T) {
	s, err := ParseCSV(smiOutLimit, time.Now())
	if err != nil || len(s) != 3 {
		t.Fatalf("%v %v", err, s)
	}
	if s[0].PowerLimitW != 100 || s[0].PowerMaxW != 250 || s[0].PowerW != 71.4 {
		t.Fatalf("%+v", s[0])
	}
	if s[2].PowerLimitW != -1 || s[2].PowerMaxW != -1 {
		t.Fatalf("N/A limit must be -1: %+v", s[2])
	}
}

func TestParseCSVWithoutLimitColumnsGivesMinusOne(t *testing.T) {
	s, err := ParseCSV(smiOut, time.Now())
	if err != nil || len(s) != 4 || s[0].PowerLimitW != -1 || s[0].PowerMaxW != -1 {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestParseCSVRejectsOddColumnCounts(t *testing.T) {
	if _, err := ParseCSV("0, 00000000:03:00.0, 1, 2, 3, 4, 100.00\n", time.Now()); err == nil {
		t.Fatal("7 columns accepted")
	}
}

func queryOf(args []string) string { return strings.Join(args, " ") }

// A driver that rejects enforced.power.limit: the collector falls back to
// power.limit, and keeps using it (no extra failing call every round).
func TestQueryFallsBackToPowerLimitAndSticks(t *testing.T) {
	var queries []string
	c := &Collector{
		AERPath: "-",
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			q := queryOf(args)
			queries = append(queries, q)
			if strings.Contains(q, "enforced.power.limit") {
				return nil, errors.New("exit status 2: Field \"enforced.power.limit\" is not a valid field to query.")
			}
			return []byte(smiOutLimit), nil
		},
	}
	s := c.Once(context.Background())
	if s.Err != "" || len(s.Samples) != 3 || s.Samples[0].PowerLimitW != 100 {
		t.Fatalf("%+v", s)
	}
	if len(queries) != 2 || !strings.Contains(queries[1], ",power.limit,power.max_limit") {
		t.Fatalf("%v", queries)
	}
	c.Once(context.Background())
	if len(queries) != 3 || !strings.Contains(queries[2], ",power.limit,power.max_limit") {
		t.Fatalf("second round must go straight to power.limit: %v", queries)
	}
}

func TestQueryFallsBackToBaseFields(t *testing.T) {
	c := &Collector{
		AERPath: "-",
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if strings.Contains(queryOf(args), "power.") && strings.Contains(queryOf(args), "limit") {
				return nil, errors.New("not a valid field")
			}
			return []byte(smiOut), nil
		},
	}
	s := c.Once(context.Background())
	if s.Err != "" || len(s.Samples) != 4 || s.Samples[0].PowerLimitW != -1 {
		t.Fatalf("%+v", s)
	}
}

// A dead nvidia-smi still reports the error, after trying every tier.
func TestQueryAllTiersFail(t *testing.T) {
	calls := 0
	c := &Collector{AERPath: "-", Run: func(context.Context, string, ...string) ([]byte, error) {
		calls++
		return nil, errors.New("not found")
	}}
	s := c.Once(context.Background())
	if !strings.Contains(s.Err, "nvidia-smi") || calls != 3 {
		t.Fatalf("calls %d %+v", calls, s)
	}
}
