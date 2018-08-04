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
    // Do our best to get a representation of an Item that can be displayed
    Render() RenderItem
}

type RenderItem struct {
    PubDate, Title, Description, Guid, Url, Enclosure string
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
    guid := tryContent(item.src, "guid")
    if len(guid) > 0 {
        return guid, nil
    }

    // no guid tag? just concat title and link and hope it's unique
    title := tryContent(item.src, "title")
    link := tryContent(item.src, "link")
    if len(link) == 0 || len(title) == 0 {
        return "", errors.New("can't build a guid")
    }
    return title + " - " + link, nil
}

func (item *RssItem) String() string {
    return item.src.String()
}

func (item *RssItem) Node() xml.Node {
    return item.src
}

func (item *RssItem) Render() RenderItem {
    pubDate, _ := item.PubDate()

    titletxt := tryContent(item.Node(), "title")
    if len(titletxt) == 0 {
        titletxt = tryContent(item.Node(), "description")
        if len(titletxt) > 150 {
            titletxt = titletxt[0:147] + "..."
        }
    }
    return RenderItem{
        pubDate.Format("2006-02-01"),
        titletxt,
        tryContent(item.src, "description"),
        tryContent(item.src, "guid"),
        tryContent(item.src, "link"),
        tryAttr(item.src, "enclosure", "url"),
    }
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

func (item *AtomItem) Render() RenderItem {
    pubDate, _ := item.PubDate()
    desc := tryContent(item.src, "content")
    if len(desc) == 0 {
        desc = tryContent(item.src, "summary")
    }
    id := tryContent(item.src, "id")
    enclosure := ""
    encTags, err := item.src.Search("link")
    if err == nil && len(encTags) > 0 {
        for _, tag := range encTags {
            rel, found := tag.Attributes()["rel"]
            if found && rel.Value() == "enclosure" {
                enclosure = tag.Attributes()["href"].Value()
                break
            }
        }
    }
    return RenderItem{
        pubDate.Format("2006-02-01"),
        tryContent(item.src, "title"),
        desc,
        id,
        id,
        enclosure,
    }
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
    switch (it.Root().Name()) {
    case "item":
        return &RssItem{it.Root()}, nil
    case "entry":
        return &AtomItem{it.Root()}, nil
    default:
        break
    }
    return nil, errors.New("Couldn't detect feed type")
}

func zeroDate() time.Time {
    return time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
}

func tryContent(node xml.Node, tagname string) string {
    // return the text content of that tag if it exists, "" if it doesn't
    tag, err := node.Search(tagname)
    if err == nil && len(tag) > 0 {
        return tag[0].Content()
    }
    return ""
}

func tryAttr(node xml.Node, tagname string, attr string) string {
    tags, err := node.Search(tagname)
    if err != nil || len(tags) == 0 {
        return ""
    }
    for _, tag := range tags {
        val, found := tag.Attributes()[attr]
        if found {
            return val.Value()
        }
    }
    return ""
}
