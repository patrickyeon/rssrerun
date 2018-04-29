package rssrerun

import (
    "errors"
    "time"

    "github.com/jbowtie/gokogiri"
    "github.com/jbowtie/gokogiri/xml"
)

//  Interface for a single feed item/entry. I've normalized on using RSS terms
// (hence calling out items and pubdates), and we only manipulate the bare
// essential (publication date and guid, really).
type Item interface {
    // return the published date of the item
    PubDate() (time.Time, error)
    // change the published date (this his how we do the rerun later)
    SetPubDate(date time.Time) (error)
    // get the guid, or make one up
    Guid() (string, error)
    // render the item as a string
    String() string
    // the actual, parsed, xml.Node of the document
    Node() xml.Node
}

type RssItem struct {
    src xml.Node
}

func (item *RssItem) PubDate() (time.Time, error) {
    for _, str := range []string{"pubDate", "pubdate", "PubDate", "PUBDATE"} {
        d, err := item.src.Search(str)
        if err == nil && len(d) > 0 {
            return parseDate(d[0].Content())
        }
    }
    return zeroDate(), errors.New("no pubdate")
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
    // no guid tag? just concat title and link and hope it's unique
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

func (item *RssItem) Node() xml.Node {
    return item.src
}


type AtomItem struct {
    src xml.Node
}

func (item *AtomItem) PubDate() (time.Time, error) {
    published, err := getChild(item.src, xpath("published"))
    if err != nil {
        return zeroDate(), err
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

func (item *AtomItem) Node() xml.Node {
    return item.src
}


func parseDate(s string) (time.Time, error) {
    for _, typ := range(dateTypes) {
        date, err := time.Parse(typ, s)
        if err == nil {
            return date, nil
        }
    }
    return zeroDate(), errors.New("invalid date format")
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

// Given an array of bytes, parse them as an RSS item
func MkItem(s []byte) (Item, error) {
    it, err := gokogiri.ParseXml(s)
    if err != nil {
        return nil, err
    }
    return &RssItem{it.Root()}, nil
}

func zeroDate() time.Time {
    return time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
}
