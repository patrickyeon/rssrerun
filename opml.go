package rssrerun

import (
    "github.com/moovweb/gokogiri"
)

type Outline struct {
    Name string
    Url string
}

type Opml struct {
    Title string
    Outlines []Outline
}

func ParseOpml(b []byte) (Opml, error) {
    parsed, err := gokogiri.ParseXml(b)
    if err != nil {
        return Opml{}, err
    }
    ret := Opml{}
    title, _ := parsed.Root().Search("//head/title")
    ret.Title = title[0].FirstChild().String()
    items, _ := parsed.Root().Search("//outline")
    outlines := make([]Outline, len(items))
    i := 0
    for _, item := range items {
        name := item.Attribute("text")
        url := item.Attribute("xmlUrl")
        if name != nil && url != nil {
            outlines[i] = Outline{name.String(), url.String()}
            i++
        }
    }
    ret.Outlines = outlines[:i]
    return ret, nil
}
