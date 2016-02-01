package rssmangle

import (
    "time"
    "rss-rerun/datesource"
    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

type Feed struct {
    Root xml.Node
    Items []xml.Node
    d *datesource.DateSource
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
        pd[0].SetContent(date.Format(time.RFC822))
    }
    return nil
}
