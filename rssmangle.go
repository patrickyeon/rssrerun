package rssrerun

import (
    "errors"
    "time"
    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

var (
    dateTypes = []string {time.RFC822, time.RFC822Z,
                          time.RFC1123, time.RFC1123Z}
)

type rawFeed interface {
    MakeItems() []Item
    Wrapper() []byte
    Bytes() []byte
    BytesWithItems(items []Item) []byte
}

type Feed struct {
    Items []Item

    raw rawFeed
    d *DateSource
}

func (f *Feed) Bytes() []byte {
    return f.BytesWithItems(f.Items)
}
func (f *Feed) BytesWithItems(items []Item) []byte {
    return f.raw.BytesWithItems(items)
}
func (f *Feed) Wrapper() []byte {
    return f.raw.Wrapper()
}
func (f *Feed) ShiftedAt(n int, t time.Time) ([]Item, error) {
    // if item N is after time t, we want items (N-n-1 .. N-1) and then shift
    ndays := f.d.DatesInRange(f.d.StartDate, t)
    // n is the number of items we want
    // ndays is the number of rerun episodes between start and t
    // (ndays - n) < 0 means they're asking for more than have rerun yet.
    //   that's ok, we need to give them fewer though
    // (ndays - n) > 0 means we need to skip the first (ndays - n) reruns
    // once they're skipped, one-for-one item and .NextDate()
    nskip := ndays - n
    if nskip < 0 {
        nskip = 0
    }
    if nskip > len(f.Items) {
        return nil, errors.New("too old")
    }
    f.d.lastDate = f.d.StartDate.AddDate(0, 0, -1)
    f.d.SkipForward(nskip)

    nret := n
    if nret + nskip > len(f.Items) {
        nret = len(f.Items) - nskip
    }
    ret := make([]Item, nret)
    for i := 0; i < nret; i++ {
        // last -1 needed because its[len(its) - x] is (x - 1)'th from the back
        ret[nret - i - 1] = f.Items[len(f.Items) - (nskip + i) - 1]
        nd, _ := f.d.NextDate()
        ret[nret - i - 1].SetPubDate(nd)
    }
    return ret, nil
}

type RssFeed struct {
    root xml.Node
    items []xml.Node
}

func (f *RssFeed) MakeItems() []Item {
    ret := make([]Item, len(f.items))
    for i, it := range f.items {
        ret[i] = &RssItem{it}
    }
    return ret
}

func newRssRaw(doc xml.Document) (*RssFeed, error) {
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
    if len(f.items) > 0 {
        f.items[0].InsertBefore(doc.CreateElementNode("item"))
    }
    for _, item := range f.items {
        item.Unlink()
    }
    return f, nil
}

func (f *RssFeed) Wrapper() []byte {
    return f.root.ToBuffer(nil)
}

func (f *RssFeed) Bytes() []byte {
    return f.bytesWithNodes(f.items)
}

func (f *RssFeed) BytesWithItems(items []Item) []byte {
    its := make([]xml.Node, len(items))
    for i, item := range items {
        its[i] = item.Node()
    }
    return f.bytesWithNodes(its)
}

func (f *RssFeed) bytesWithNodes(nodes []xml.Node) []byte {
    return insertForRender(f.root, nodes, "channel/item")
}

type AtomFeed struct {
    root xml.Node
    entries []xml.Node
    d *DateSource
}

func (a *AtomFeed) MakeItems() []Item {
    ret := make([]Item, len(a.entries))
    for i, it := range a.entries {
        ret[i] = &AtomItem{it}
    }
    return ret
}

func (a *AtomFeed) Bytes() []byte {
    return a.bytesWithNodes(a.entries)
}

func (f *AtomFeed) BytesWithItems(items []Item) []byte {
    its := make([]xml.Node, len(items))
    for i, item := range items {
        its[i] = item.Node()
    }
    return f.bytesWithNodes(its)
}

func (a *AtomFeed) bytesWithNodes(nodes []xml.Node) []byte {
    return insertForRender(a.root, nodes, "//*[local-name()='entry']")
}

func (a *AtomFeed) Wrapper() []byte {
    return a.root.ToBuffer(nil)
}

func newAtomRaw(doc xml.Document)  (*AtomFeed, error) {
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
    if len(a.entries) > 0 {
        first := a.entries[0]
        placeholder := doc.CreateElementNode("entry")
        namespace := first.Namespace()
        for _, ns := range first.DeclaredNamespaces() {
            if ns.Uri == namespace {
                placeholder.SetNamespace(ns.Prefix, ns.Uri)
            }
        }
        first.InsertBefore(placeholder)
    }
    for _, entry := range a.entries {
        entry.Unlink()
    }

    return a, nil
}

func NewFeed(t []byte, d *DateSource) (*Feed, error) {
    doc, err := gokogiri.ParseXml(t)
    if err != nil {
        return nil, err
    }
    rss, rssErr := newRssRaw(doc)
    if rssErr == nil {
        return &Feed{rss.MakeItems(), rss, d}, nil
    }
    atom, atomErr := newAtomRaw(doc)
    if atomErr == nil {
        return &Feed{atom.MakeItems(), atom, d}, nil
    }
    return nil, errors.New("Couldn't parse feed as RSS: \"" + rssErr.Error() +
                           "\", nor as ATOM: \"" + atomErr.Error() + "\"")
}

func insertForRender(parent xml.Node, children []xml.Node, where string) []byte {
    placeholder, err := parent.Search(where)
    if err != nil {
        // weird, the placeholder isn't there
        return parent.ToBuffer(nil)
    }
    lastchild := placeholder[0]
    for _, child := range children {
        if err = lastchild.AddNextSibling(child); err != nil {
            return nil
        }
        lastchild = child
    }
    placeholder[0].Unlink()
    retval := parent.ToBuffer(nil)
    if err  = children[0].AddPreviousSibling(placeholder); err != nil {
        return nil
    }
    for _, child := range children {
        child.Unlink()
    }
    return retval
}
