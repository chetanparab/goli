package usage

import (
	"errors"
	"testing"
	"time"
)

func TestRecordBoostUnused(t *testing.T) {
	s := Open(t.TempDir())
	if _, err := s.Unused([]string{"a"}, time.Now()); !errors.Is(err, ErrNoHistory) {
		t.Fatalf("before any record: %v", err)
	}
	for i := 0; i < 10; i++ {
		s.Record("daily")
	}
	s.Record("rare")
	boost := s.Boost()
	if boost("daily") <= boost("rare") || boost("rare") <= 0 || boost("never") != 0 || boost("daily") > 0.12 {
		t.Errorf("boost daily=%.3f rare=%.3f never=%.3f", boost("daily"), boost("rare"), boost("never"))
	}
	unused, err := s.Unused([]string{"daily", "rare", "never"}, time.Now().Add(-time.Hour))
	if err != nil || len(unused) != 1 || unused[0] != "never" {
		t.Errorf("unused = %v, %v", unused, err)
	}
	s.Forget("daily")
	if s.All()["daily"].Count != 0 || s.All()["rare"].Count != 1 {
		t.Errorf("after forget: %+v", s.All())
	}
}
