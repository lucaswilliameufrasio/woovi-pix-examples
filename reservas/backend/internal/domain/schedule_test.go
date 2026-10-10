package domain

import (
	"testing"
	"time"
)

func TestAvailableSlotsPreservesDSTGapAndFold(t *testing.T) {
	resource := Resource{TimeZone: "America/New_York", DurationMinutes: 30, BufferMinutes: 15, SlotIncrementMinutes: 15}
	windows := []OpeningWindow{{Weekday: time.Sunday, OpensAt: "00:00", ClosesAt: "04:00"}}
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	springSlots, err := AvailableSlots(resource, windows, "2026-03-08", now, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range springSlots {
		local := slot.StartsAt.In(mustLoadLocation(t, resource.TimeZone))
		if local.Hour() == 2 {
			t.Fatalf("nonexistent spring-forward local time was offered: %s", slot.LocalLabel)
		}
	}

	resource.DurationMinutes = 15
	resource.BufferMinutes = 0
	fallSlots, err := AvailableSlots(resource, windows, "2026-11-01", now, nil)
	if err != nil {
		t.Fatal(err)
	}
	foldStarts := make([]time.Time, 0, 2)
	for _, slot := range fallSlots {
		local := slot.StartsAt.In(mustLoadLocation(t, resource.TimeZone))
		if local.Hour() == 1 && local.Minute() == 30 {
			foldStarts = append(foldStarts, slot.StartsAt)
		}
	}
	if len(foldStarts) != 2 || !foldStarts[1].After(foldStarts[0]) || foldStarts[1].Sub(foldStarts[0]) != time.Hour {
		t.Fatalf("expected two distinct 01:30 occurrences during fall-back, got %v", foldStarts)
	}
}

func TestAvailableSlotsHonorsDurationBufferAndAdjacency(t *testing.T) {
	resource := Resource{TimeZone: "America/Sao_Paulo", DurationMinutes: 30, BufferMinutes: 15, SlotIncrementMinutes: 15}
	windows := []OpeningWindow{{Weekday: time.Monday, OpensAt: "09:00", ClosesAt: "11:00"}}
	location := mustLoadLocation(t, resource.TimeZone)
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	first := time.Date(2026, time.January, 5, 9, 0, 0, 0, location)
	busy := []BusyInterval{{StartsAt: first.UTC(), OccupiedUntil: first.Add(45 * time.Minute).UTC()}}
	slots, err := AvailableSlots(resource, windows, "2026-01-05", now, busy)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range slots {
		if slot.StartsAt.Equal(first.UTC()) || slot.StartsAt.Equal(first.Add(15*time.Minute).UTC()) || slot.StartsAt.Equal(first.Add(30*time.Minute).UTC()) {
			t.Fatalf("overlapping slot should not be available: %s", slot.LocalLabel)
		}
	}
	if !containsStart(slots, first.Add(45*time.Minute).UTC()) {
		t.Fatal("slot starting exactly when the prior buffer ends should remain available")
	}
}

func TestAvailableSlotsRejectsInvalidDatesAndSchedules(t *testing.T) {
	resource := Resource{TimeZone: "No/Such_Zone", DurationMinutes: 30, SlotIncrementMinutes: 15}
	if _, err := AvailableSlots(resource, nil, "2026-01-01", time.Now(), nil); err == nil {
		t.Fatal("expected invalid timezone to be rejected")
	}
	resource.TimeZone = "UTC"
	if _, err := AvailableSlots(resource, nil, "2026-02-30", time.Now(), nil); err == nil {
		t.Fatal("expected invalid date to be rejected")
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func containsStart(slots []Slot, start time.Time) bool {
	for _, slot := range slots {
		if slot.StartsAt.Equal(start) {
			return true
		}
	}
	return false
}
