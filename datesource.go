package rssrerun

import (
    "errors"
    "time"
)

//  We want to set a schedule for reruns based on a start date and a day-of-week
// schedule (eg, Monday-Wednesday-Friday). This lets us do that and do things
// like iterate through the dates.
type DateSource struct {
    StartDate time.Time
    Schedule []time.Weekday
    lastDate time.Time
}

func (d *DateSource) containsDay(day time.Time) bool {
    // only checks the schedule, not for startdate
    weekday := day.Weekday()
    for _, wd := range d.Schedule {
        if wd == weekday {
            return true
        }
    }
    return false
}

func NewDateSource(start time.Time, schedule []time.Weekday) *DateSource {
    d := new(DateSource)
    d.StartDate = time.Date(start.Year(), start.Month(), start.Day(),
                            0, 0, 0, 0, time.UTC)
    d.Schedule = schedule
    // initialize to a day before the startdate so that we catch it too
    d.lastDate = d.StartDate.AddDate(0, 0, -1)
    return d
}

// What is the next date on the schedule for our `DateSource`?
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
        if d.containsDay(d.lastDate) {
        // will eventually work, we already know d.Schedule is not empty.
            return d.lastDate, nil
        }
        d.lastDate = d.lastDate.AddDate(0, 0, 1)
    }
}

// skip forward by `nDays` *scheduled* days
func (d *DateSource) SkipForward(nDays int) {
    for nDays != 0 {
        if nDays > 0 {
            _, _ = d.NextDate()
            nDays--
        } else {
            for {
                d.lastDate = d.lastDate.AddDate(0, 0, -1)
                if d.containsDay(d.lastDate) {
                    break
                }
            }
            nDays++
        }
    }
}

// how many days are there between `from` and `to`, inclusive?
// TODO or does it not include `to`? Also, is this actually correct?
func (d *DateSource) DatesInRange(from, to time.Time) int {
    if from.After(to) {
        return 0
    }
    storelast := d.lastDate
    d.lastDate = from
    nDates := 0
    for when := from; when.Before(to); when, _ = d.NextDate() {
        nDates++
    }
    d.lastDate = storelast
    // if from is not a date on our schedule, we have counted one too many
    // TODO I don't actually trust this.
    if !d.containsDay(from) {
        nDates -= 1
    }
    return nDates
}
