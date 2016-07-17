package rssrerun

import (
    "time"
    "testing"
)

func StartDate() time.Time {
    // Arbitrarily, March 19, 2015 (a Thursday)
    return time.Date(2015, time.March, 19, 0, 0, 0, 0, time.UTC)
}

func cmpDays(t1, t2 time.Time) int {
    // t1 < t2 ? -1 : (t1 > t2 ? 1 : 0)
    if t1.Year() < t2.Year() || (t1.Year() == t2.Year() &&
                                 t1.YearDay() < t2.YearDay()) {
        return -1
    }
    if t1.Year() > t2.Year() || (t1.Year() == t2.Year() &&
                                 t1.YearDay() > t2.YearDay()) {
        return 1
    }
    return 0
}

func TestNewDateSource(t *testing.T) {
	dsrc := NewDateSource(StartDate(), nil)
    if dsrc == nil {
        t.Fail()
    }
}

func TestNoDatesOnEmptySched(t *testing.T) {
    dsrc := NewDateSource(StartDate(), nil)
    _, err := dsrc.NextDate()
    if err == nil {
        t.Fail()
    }
}

func TestDatesAlwaysIncrement(t *testing.T) {
    dsrc := NewDateSource(StartDate(), []time.Weekday{time.Sunday})
    lastdate := StartDate().AddDate(0, 0, -1)
    for i := 0; i < 100; i++ {
        when, err := dsrc.NextDate()
        if err != nil {
            t.Error("couldn't get a NextDate()")
        }
        if cmpDays(when, lastdate) <= 0 {
            t.Error("not returning increasing date")
        }
        lastdate = when
    }
}

func TestNextDateOnSchedule(t *testing.T) {
    dsrc := NewDateSource(StartDate(),
                          []time.Weekday{time.Sunday, time.Tuesday})
    for i := 0; i < 100; i++ {
        when, err := dsrc.NextDate()
        if err != nil {
            t.Error("couldn't get a NextDate()")
        }
        if when.Weekday() != time.Sunday && when.Weekday() != time.Tuesday {
            t.Error("NextDate() outside of schedule")
        }
    }
}

func TestFFDate(t *testing.T) {
    skipsrc := NewDateSource(StartDate(),
                             []time.Weekday{time.Sunday, time.Tuesday})
    incsrc := NewDateSource(StartDate(),
                            []time.Weekday{time.Sunday, time.Tuesday})
    for i := 0; i < 10; i++ {
        _, _ = incsrc.NextDate()
    }
    skipsrc.SkipForward(10)

    dayIncrementing, _ := incsrc.NextDate()
    daySkipping, _ := skipsrc.NextDate()
    if cmpDays(daySkipping, dayIncrementing) < 0 {
        t.Error("SkipForward() not forward enough")
    } else if cmpDays(daySkipping, dayIncrementing) > 0 {
        t.Error("SkipForward() too far forward")
    }
}

func TestFFDateBackwards(t *testing.T) {
    dsrc := NewDateSource(StartDate(),
                          []time.Weekday{time.Sunday, time.Tuesday})
    when, _ := dsrc.NextDate()
    dsrc.SkipForward(-1)
    skipped, _ := dsrc.NextDate()
    if cmpDays(skipped, when) < 0 {
        t.Error("Too far back in time")
    }
    if cmpDays(skipped, when) > 0 {
        t.Error("Too far forward in time")
    }
}

func TestFFPastStartDate(t *testing.T) {
    dsrc := NewDateSource(StartDate(),
                          []time.Weekday{time.Sunday, time.Tuesday})
    first, _ := dsrc.NextDate()
    dsrc.SkipForward(-100)
    reset, _ := dsrc.NextDate()
    if cmpDays(reset, first) != 0 {
        t.Error("skip backwards didn't bring back to start")
    }
}

func TestDatesInRange(t *testing.T) {
    dsrc := NewDateSource(StartDate(),
                          []time.Weekday{time.Sunday, time.Tuesday})
    past, _ := dsrc.NextDate()
    dsrc.SkipForward(10)
    future, _ := dsrc.NextDate()
    nItems := dsrc.DatesInRange(past, future)
    // there are the ten days skipped forward, and then the one more from
    //  dsrc.NextDate()
    if nItems != 11 {
        t.Errorf("Incorrect range count, exp: 11, got:%d", nItems)
    }
}

func TestDatesInRangeOffset(t *testing.T) {
    // important, this is a Wednesday
    start := time.Date(2015, time.March, 19, 0, 0, 0, 0, time.UTC)
    dsrc := NewDateSource(start,
                          []time.Weekday{time.Sunday, time.Tuesday})
    if nDates := dsrc.DatesInRange(start, start.AddDate(0, 0, 7)); nDates != 2 {
        t.Errorf("Expected 2 dates in range, got %d", nDates);
    }
}
