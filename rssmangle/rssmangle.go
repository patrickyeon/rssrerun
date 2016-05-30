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
    f.items, err = doc.Root().Search("//channel//item")
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
    if len(f.items) == 0 {
        return nil, errors.New("no items in feed")
    }

    f.TimeShift()
    i := -1
    for j, it := range f.Items() {
        d, err := it.PubDate()
        //d, err := f.pubDate(&it)
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
    if realend := len(f.items) - 1; realend < end {
        end = realend
    }

    return f.Items()[i : end], nil
}

type AtomFeed struct {
    root xml.Node
    entries []xml.Node
    d *datesource.DateSource
    timeshifted bool
    dtInd int
}

func (a *AtomFeed) TimeShift() error {
    return errors.New("not implemented")
}
func (a *AtomFeed) Items() []Item {
    return nil
}
func (a *AtomFeed) Item(n int) Item {
    return nil
}
func (a *AtomFeed) LatestAt(n int, t time.Time) ([]Item, error) {
    return nil, errors.New("not implemented")
}
func (a *AtomFeed) Bytes() []byte {
    return nil
}

func newAtomFeed(doc xml.Document, d *datesource.DateSource) (*AtomFeed, error) {
    return nil, errors.New("atom parse not implemented")
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
