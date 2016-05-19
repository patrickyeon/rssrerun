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

type Item interface {
    PubDate() (time.Time, error)
    NewPubDate(date time.Time) (error)
    Guid() (string, error)
    String() string
}

type Feed interface {
    TimeShift() error

    Items() []Item
    Item(n int) Item
    LatestAt(n int, t time.Time) ([]Item, error)

    Bytes() []byte

}

type RssItem struct {
    src xml.Node
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

func (item *RssItem) String() string {
    return item.src.String()
}

func (f *RssFeed) Item(n int) Item {
    return &RssItem{f.items[n]}
}

func (item *RssItem) Guid() (string, error) {
    // come on, let's hope for a proper guid
    gtag, err := item.src.Search("guid")
    if err == nil && len(gtag) > 0 {
        return gtag[0].Content(), nil
    }
    // no guid tag? just concat title and link
    title, err := item.src.Search("title")
    if err != nil {
        return "", err
    }
    link, err := item.src.Search("link")
    if err != nil {
        return "", err
    }
    if len(link) == 0 || len(title) == 0 {
        return "", errors.New("can't build a guid")
    }
    return title[0].Content() + " - " + link[0].Content(), nil
}

func (item *RssItem) PubDate() (time.Time, error) {
    zdate := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
    d, err := item.src.Search("pubDate")
    if err != nil {
        return zdate, err
    }
    if len(d) != 1 {
        return zdate, errors.New("no pubdate" + string(len(d)))
    }
    return parseDate(d[0].Content())
}

func (item *RssItem) NewPubDate(date time.Time) (error) {
    return nil
}

func NewFeed(t []byte, d *datesource.DateSource) (*RssFeed, error) {
    doc, err := gokogiri.ParseXml(t)
    if err != nil {
        return nil, err
    }
    // TODO check assumptions (one channel)
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
        it := f.items[i]
        pd, err := it.Search("pubDate")
        if err != nil {
            return err
        }
        date, err := f.d.NextDate()
        if err != nil {
            return err
        }
        olddate, err := parseDate(pd[0].Content())
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

func parseDate(s string) (time.Time, error) {
    for _, typ := range(dateTypes) {
        date, err := time.Parse(typ, s)
        if err == nil {
            return date, nil
        }
    }
    zdate := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
    return zdate, errors.New("invalid date format")
}
