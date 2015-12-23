package rssmangle

import (
    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

type Node xml.Node

type Feed struct {
    Root Node
    Items []xml.Node
}

func NewFeed(t []byte) (*Feed, error) {
    doc, err := gokogiri.ParseXml(t)
    if err != nil {
        return nil, err
    }
    f := new(Feed)
    f.Root = doc.Root()
    f.Items, err = doc.Root().Search("//channel//item")
    if err != nil {
        return nil, err
    }
    return f, nil
}

func (f *Feed) Bytes() []byte {
    return f.Root.ToBuffer(nil)
}

