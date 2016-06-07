package rssmangle

import (
    "errors"
    "time"

    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

type Item interface {
    PubDate() (time.Time, error)
    SetPubDate(date time.Time) (error)
    Guid() (string, error)
    String() string
}

type RssItem struct {
    src xml.Node
}

func (item *RssItem) PubDate() (time.Time, error) {
    zdate := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
    for _, str := range []string{"pubDate", "pubdate", "PubDate", "PUBDATE"} {
        d, err := item.src.Search(str)
        if err == nil && len(d) > 0 {
            return parseDate(d[0].Content())
        }
    }
    return zdate, errors.New("no pubdate")
}

func (item *RssItem) SetPubDate(date time.Time) (error) {
    pdtag, err := item.src.Search("pubDate")
    if err != nil {
        return err
    }
    if len(pdtag) == 0 {
        return errors.New("no pubdate tag")
    }
    pdtag[0].SetContent(date.Format(time.RFC822))
    return nil
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

func (item *RssItem) String() string {
    return item.src.String()
}


type AtomItem struct {
    src xml.Node
}

func (item *AtomItem) PubDate() (time.Time, error) {
    published, err := getChild(item.src, xpath("published"))
    if err != nil {
        return time.Unix(0, 0), err
    }
    return parseDate(published.Content())
}

func (item *AtomItem) SetPubDate(date time.Time) error {
    published, err := getChild(item.src, xpath("published"))
    if err != nil {
        return err
    }
    return published.SetContent(date.Format(time.RFC822))
}

func (item *AtomItem) Guid() (string, error) {
    id, err := getChild(item.src, xpath("id"))
    if err != nil {
        return "", err
    }
    return id.Content(), nil
}

func (item *AtomItem) String() string {
    return item.src.String()
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

func xpath(s string) string {
    return "*[local-name()='" + s + "']"
}

func getChild(parent xml.Node, tagName string) (xml.Node, error) {
    ret, err := parent.Search(tagName)
    if err != nil {
        return nil, err
    }
    if len(ret) == 0 {
        return nil, errors.New("no <" + tagName + "> tag found")
    }
    return ret[0], nil
}

func MkItem(s []byte) (Item, error) {
    it, err := gokogiri.ParseXml(s)
    if err != nil {
        return nil, err
    }
    return &RssItem{it.Root()}, nil
}
