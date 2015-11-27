package datesource

import (
    "errors"
    "time"
)

type DateSource struct {
    StartDate time.Time
    Schedule []time.Weekday
    lastDate time.Time
}

func NewDateSource(start time.Time, schedule []time.Weekday) *DateSource {
    d := new(DateSource)
    d.StartDate = time.Date(start.Year(), start.Month(), start.Day(),
                            0, 0, 0, 0, time.UTC)
    d.Schedule = schedule
    d.lastDate = d.StartDate.AddDate(0, 0, -1)
    return d
}

func (d *DateSource) NextDate() (time.Time, error) {
    if d.Schedule == nil {
        return d.StartDate, errors.New("NextDate() on empty schedule")
    }
    if d.lastDate.Before(d.StartDate) {
        d.lastDate = d.StartDate
    } else {
		d.lastDate = d.lastDate.AddDate(0, 0, 1)
	}
    for {
        for _, wd := range d.Schedule {
            // will eventually work, we've already made sure d.Schedule is not
            //  empty.
            if d.lastDate.Weekday() == wd {
                return d.lastDate, nil
            }
        }
        d.lastDate = d.lastDate.AddDate(0, 0, 1)
    }
}

func (d *DateSource) SkipForward(nDays int) {
    // skip forward by nDays *scheduled* days
    for nDays != 0 {
        if nDays > 0 {
            _, _ = d.NextDate()
            nDays--
        } else {
            for {
                d.lastDate = d.lastDate.AddDate(0, 0, -1)
                valid := false
                for _, wd := range d.Schedule {
                    if d.lastDate.Weekday() == wd {
                        valid = true
                        break
                    }
                }
                if valid {
                    break
                }
            }
            nDays++
        }
    }
}

func (d *DateSource) DatesInRange(from, to time.Time) int {
    if from.After(to) {
        return 0
    }
    storelast := d.lastDate
    d.lastDate = from
    nDates := 0
    for when, _ := d.NextDate(); when.Before(to); when, _ = d.NextDate() {
        nDates++
    }
    d.lastDate = storelast
    return nDates
}
