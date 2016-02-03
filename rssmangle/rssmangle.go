package rssmangle

import (
    "errors"
    "time"
    "rss-rerun/datesource"
    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

type Feed struct {
    Root xml.Node
    Items []xml.Node
    d *datesource.DateSource
    timeshifted bool
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
        olddate, err := time.Parse(time.RFC822, pd[0].Content())
        if err != nil {
            return err
        }
        if olddate.After(date) {
            break
        }
        pd[0].SetContent(date.Format(time.RFC822))
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
        d, err := pubDate(&it)
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

func pubDate(n *xml.Node) (time.Time, error) {
    zdate := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
    d, err := (*n).Search("pubDate")
    if err != nil {
        return zdate, err
    }
    if len(d) != 1 {
        return zdate, errors.New("no pubdate" + string(len(d)))
    }
    ret, err := time.Parse(time.RFC822, d[0].Content())
    if err != nil {
        return zdate, err
    }
    return ret, nil
}
