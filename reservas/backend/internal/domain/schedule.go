package domain

import (
	"fmt"
	"time"
)

type Resource struct {
	ID                   string
	Name                 string
	TimeZone             string
	PriceCents           int64
	DurationMinutes      int
	BufferMinutes        int
	SlotIncrementMinutes int
	HoldSeconds          int
}

type OpeningWindow struct {
	Weekday  time.Weekday
	OpensAt  string
	ClosesAt string
}

type BusyInterval struct {
	StartsAt      time.Time
	OccupiedUntil time.Time
}

type Slot struct {
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	LocalLabel string    `json:"local_label"`
	TimeZone   string    `json:"time_zone"`
}

func AvailableSlots(resource Resource, windows []OpeningWindow, date string, now time.Time, busy []BusyInterval) ([]Slot, error) {
	location, err := time.LoadLocation(resource.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("load resource time zone: %w", err)
	}
	localDate, err := time.Parse("2006-01-02", date)
	if err != nil || localDate.Format("2006-01-02") != date {
		return nil, fmt.Errorf("date must use YYYY-MM-DD")
	}
	if resource.DurationMinutes < 1 || resource.BufferMinutes < 0 || resource.SlotIncrementMinutes < 1 {
		return nil, fmt.Errorf("resource schedule configuration is invalid")
	}
	localStart := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, location)
	localEnd := time.Date(localDate.Year(), localDate.Month(), localDate.Day()+1, 0, 0, 0, 0, location)
	step := time.Duration(resource.SlotIncrementMinutes) * time.Minute
	serviceDuration := time.Duration(resource.DurationMinutes) * time.Minute
	occupiedDuration := time.Duration(resource.DurationMinutes+resource.BufferMinutes) * time.Minute
	slots := make([]Slot, 0)
	for instant := localStart; instant.Before(localEnd); instant = instant.Add(step) {
		local := instant.In(location)
		if local.Weekday() != localDate.Weekday() || !inOpeningWindow(local, windows, resource.SlotIncrementMinutes) {
			continue
		}
		endsAt := instant.Add(serviceDuration)
		occupiedUntil := instant.Add(occupiedDuration)
		localEndTime := endsAt.In(location)
		if localEndTime.Year() != local.Year() || localEndTime.YearDay() != local.YearDay() || !endWithinOpeningWindow(localEndTime, windows, local.Weekday()) {
			continue
		}
		_, startOffset := local.Zone()
		_, endOffset := localEndTime.Zone()
		if startOffset != endOffset {
			continue
		}
		if !instant.After(now) || overlapsAny(instant, occupiedUntil, busy) {
			continue
		}
		slots = append(slots, Slot{
			StartsAt:   instant.UTC(),
			EndsAt:     endsAt.UTC(),
			LocalLabel: local.Format("Mon 02 Jan 15:04 -07:00"),
			TimeZone:   resource.TimeZone,
		})
	}
	return slots, nil
}

func inOpeningWindow(local time.Time, windows []OpeningWindow, increment int) bool {
	minuteOfDay := local.Hour()*60 + local.Minute()
	for _, window := range windows {
		if window.Weekday != local.Weekday() {
			continue
		}
		opensAt, openErr := time.Parse("15:04", window.OpensAt)
		closesAt, closeErr := time.Parse("15:04", window.ClosesAt)
		if openErr != nil || closeErr != nil {
			continue
		}
		openMinute := opensAt.Hour()*60 + opensAt.Minute()
		closeMinute := closesAt.Hour()*60 + closesAt.Minute()
		if minuteOfDay >= openMinute && minuteOfDay < closeMinute && local.Second() == 0 && local.Nanosecond() == 0 && (minuteOfDay-openMinute)%increment == 0 {
			return true
		}
	}
	return false
}

func endWithinOpeningWindow(end time.Time, windows []OpeningWindow, weekday time.Weekday) bool {
	minuteOfDay := end.Hour()*60 + end.Minute()
	for _, window := range windows {
		if window.Weekday != weekday {
			continue
		}
		closesAt, err := time.Parse("15:04", window.ClosesAt)
		if err == nil && end.Second() == 0 && end.Nanosecond() == 0 && minuteOfDay <= closesAt.Hour()*60+closesAt.Minute() {
			return true
		}
	}
	return false
}

func overlapsAny(startsAt, occupiedUntil time.Time, busy []BusyInterval) bool {
	for _, interval := range busy {
		if startsAt.Before(interval.OccupiedUntil) && interval.StartsAt.Before(occupiedUntil) {
			return true
		}
	}
	return false
}
