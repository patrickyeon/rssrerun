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

type Feed interface {
    TimeShift() error

    Items() []Item
    Item(n int) Item
    LatestAt(n int, t time.Time) ([]Item, error)

    Bytes() []byte
}

type RssFeed struct {
    root xml.Node
    items []xml.Node
    d *datesource.DateSource
    timeshifted bool
    dtInd int
}

func (f *RssFeed) Items() []Item {
    ret := make([]Item, len(f.items))
    for i, it := range f.items {
        ret[i] = &RssItem{it}
    }
    return ret
}

func (f *RssFeed) Item(n int) Item {
    return &RssItem{f.items[n]}
}

func newRssFeed(doc xml.Document, d *datesource.DateSource) (*RssFeed, error) {
    channels, err := doc.Root().Search("channel")
    if err != nil {
        return nil, err
    }
    if len(channels) == 0 {
        return nil, errors.New("No <channel> tag for RSS feed")
    }
    if len(channels) > 1 {
        return nil, errors.New("Too many <channel> tags for RSS feed")
    }

    f := new(RssFeed)
    f.root = doc.Root()
    f.items, err = doc.Root().Search("channel/item")
    if err != nil {
        return nil, err
    }
    f.d = d
    f.dtInd = 0
    return f, nil
}

func (f *RssFeed) Bytes() []byte {
    return f.root.ToBuffer(nil)
}

func (f *RssFeed) TimeShift() error {
    if f.timeshifted {
        return nil
    }
    for i := (len(f.items) - 1); i >= 0; i-- {
        it := f.Item(i)
        olddate, err := it.PubDate()
        if err != nil {
            return err
        }
        date, err := f.d.NextDate()
        if err != nil {
            return err
        }
        if olddate.After(date) {
            break
        }
        if err = it.SetPubDate(date); err != nil {
            return err
        }
    }
    f.timeshifted = true
    return nil
}

func (f *RssFeed) LatestAt(n int, t time.Time) ([]Item, error) {
    return latestAt(f, n, t)
}

type AtomFeed struct {
    root xml.Node
    entries []xml.Node
    d *datesource.DateSource
    timeshifted bool
    dtInd int
}

func (a *AtomFeed) TimeShift() error {
    if a.timeshifted {
        return nil
    }
    for i := (len(a.entries) - 1); i >= 0; i-- {
        it := a.Item(i)
        olddate, err := it.PubDate()
        if err != nil {
            return err
        }
        date, err := a.d.NextDate()
        if err != nil {
            return err
        }
        if olddate.After(date) {
            break
        }
        if err = it.SetPubDate(date); err != nil {
            return err
        }
    }
    a.timeshifted = true
    return nil
}

func (a *AtomFeed) Items() []Item {
    ret := make([]Item, len(a.entries))
    for i, it := range a.entries {
        ret[i] = &AtomItem{it}
    }
    return ret
}

func (a *AtomFeed) Item(n int) Item {
    return &AtomItem{a.entries[n]}
}

func (a *AtomFeed) LatestAt(n int, t time.Time) ([]Item, error) {
    return latestAt(a, n, t)
}

func (a *AtomFeed) Bytes() []byte {
    return a.root.ToBuffer(nil)
}

func newAtomFeed(doc xml.Document, d *datesource.DateSource) (*AtomFeed, error) {
    if doc.Root().Name() != "feed" {
        return nil, errors.New("<feed> tag missing or not root for Atom feed")
    }
    a := new(AtomFeed)
    a.root = doc.Root()
    var err error
    a.entries, err = doc.Root().Search("//*[local-name()='entry']")
    if err != nil {
        return nil, err
    }
    a.d = d
    a.timeshifted = false
    a.dtInd = 0

    return a, nil
}

func NewFeed(t []byte, d *datesource.DateSource) (Feed, error) {
    doc, err := gokogiri.ParseXml(t)
    if err != nil {
        return nil, err
    }
    rss, rssErr := newRssFeed(doc, d)
    if rssErr == nil {
        return rss, nil
    }
    atom, atomErr := newAtomFeed(doc, d)
    if atomErr == nil {
        return atom, nil
    }
    return nil, errors.New("Couldn't parse feed as RSS: \"" + rssErr.Error() +
                           "\", nor as ATOM: \"" + atomErr.Error() + "\"")
}

func latestAt(f Feed, n int, t time.Time) ([]Item, error) {
    if len(f.Items()) == 0 {
        return nil, errors.New("no items in feed")
    }

    f.TimeShift()
    i := -1
    for j, it := range f.Items() {
        d, err := it.PubDate()
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
    if realend := len(f.Items()) - 1; realend < end {
        end = realend
    }

    return f.Items()[i : end], nil
}
