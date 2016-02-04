package rssmangle

import (
    "errors"
    "time"
    "rss-rerun/datesource"
    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

var (
    dateTypes = []string {time.RFC822, time.RFC822Z,
                          time.RFC1123, time.RFC1123Z}
)

type Feed struct {
    Root xml.Node
    Items []xml.Node
    d *datesource.DateSource
    timeshifted bool
    dtInd int
}

func NewFeed(t []byte, d *datesource.DateSource) (*Feed, error) {
    doc, err := gokogiri.ParseXml(t)
    if err != nil {
        return nil, err
    }
    // TODO check assumptions (one channel)
    f := new(Feed)
    f.Root = doc.Root()
    f.Items, err = doc.Root().Search("//channel//item")
    if err != nil {
        return nil, err
    }
    f.d = d
    f.dtInd = 0
    return f, nil
}

func (f *Feed) Bytes() []byte {
    return f.Root.ToBuffer(nil)
}

func (f *Feed) TimeShift() error {
    if f.timeshifted {
        return nil
    }
    for i := (len(f.Items) - 1); i >= 0; i-- {
        it := f.Items[i]
        pd, err := it.Search("pubDate")
        if err != nil {
            return err
        }
        date, err := f.d.NextDate()
        if err != nil {
            return err
        }
        olddate, err := f.parseDate(pd[0].Content())
        if err != nil {
            return err
        }
        if olddate.After(date) {
            break
        }
        pd[0].SetContent(date.Format(dateTypes[f.dtInd]))
    }
    f.timeshifted = true
    return nil
}

func (f *Feed) LatestAt(n int, t time.Time) ([]xml.Node, error) {
    if len(f.Items) == 0 {
        return nil, errors.New("no items in feed")
    }

    f.TimeShift()
    i := -1
    for j, it := range f.Items {
        d, err := f.pubDate(&it)
        if err != nil {
            return nil, err
        }
        if !d.After(t) {
            i = j
            break
        }
    }
    if i == -1 {
        // all of the items are after time t
        return nil, errors.New("latest time comes before oldest item")
    }

    end := i + n
    if realend := len(f.Items) - 1; realend < end {
        end = realend
    }

    return f.Items[i : end], nil
}

func (f *Feed) parseDate(s string) (time.Time, error) {
    if f.dtInd < 0 || len(dateTypes) >= f.dtInd {
        f.dtInd = 0
    }
    startdt := f.dtInd
    for {
        date, err := time.Parse(dateTypes[f.dtInd], s)
        if err == nil {
            return date, nil
        }
        f.dtInd++
        if f.dtInd >= len(dateTypes) {
            f.dtInd = 0
        }
        if f.dtInd == startdt {
            zdate := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
            return zdate, errors.New("invalid date format")
        }
    }
}

func (f *Feed) pubDate(n *xml.Node) (time.Time, error) {
    zdate := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
    d, err := (*n).Search("pubDate")
    if err != nil {
        return zdate, err
    }
    if len(d) != 1 {
        return zdate, errors.New("no pubdate" + string(len(d)))
    }
    return f.parseDate(d[0].Content())
}
