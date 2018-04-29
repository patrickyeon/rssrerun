package rssrerun

import (
    "errors"
    "time"
    "github.com/jbowtie/gokogiri"
    "github.com/jbowtie/gokogiri/xml"
)

var (
    dateTypes = []string {time.RFC822, time.RFC822Z,
    time.RFC1123, time.RFC1123Z}
)

type Feed interface {
    Wrapper() []byte
    Bytes() []byte
    BytesWithItems(items []Item) []byte
    Items() []Item
    ShiftedAt(n int, t time.Time) ([]Item, error)
}

func univShiftedAt(n int, t time.Time, f Feed, d *DateSource) ([]Item, error) {
    items := f.Items()
    // if item N is after time t, we want items (N-n-1 .. N-1) and then shift
    ndays := d.DatesInRange(d.StartDate, t)
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
    if nskip > len(items) {
        return nil, errors.New("too old")
    }
    d.lastDate = d.StartDate.AddDate(0, 0, -1)
    d.SkipForward(nskip)

    nret := n
    if nret + nskip > len(items) {
        nret = len(items) - nskip
    }
    ret := make([]Item, nret)
    for i := 0; i < nret; i++ {
        // last -1 needed because its[len(its) - x] is (x - 1)'th from the back
        ret[nret - i - 1] = items[len(items) - (nskip + i) - 1]
        nd, _ := d.NextDate()
        ret[nret - i - 1].SetPubDate(nd)
    }
    return ret, nil
}

type RssFeed struct {
    root xml.Node
    itemNodes []xml.Node
    items []Item
    d *DateSource
}

func (f *RssFeed) ShiftedAt(n int, t time.Time) ([]Item, error) {
    return univShiftedAt(n, t, f, f.d)
}

func (f *RssFeed) Items() []Item {
    return f.items
}

// TODO error on already existing here?
func (f *RssFeed) makeItems() {
    f.items = make([]Item, len(f.itemNodes))
    for i, it := range f.itemNodes {
        f.items[i] = &RssItem{it}
    }
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
    f.itemNodes, err = doc.Root().Search("channel/item")
    if err != nil {
        return nil, err
    }
    if len(f.itemNodes) > 0 {
        f.itemNodes[0].InsertBefore(doc.CreateElementNode("item"))
    }
    for _, item := range f.itemNodes {
        item.Unlink()
    }
    return f, nil
}

func (f *RssFeed) Wrapper() []byte {
    return f.root.ToBuffer(nil)
}

func (f *RssFeed) Bytes() []byte {
    return f.bytesWithNodes(f.itemNodes)
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
    items []Item
    d *DateSource
}

func (a *AtomFeed) makeItems() {
    a.items = make([]Item, len(a.entries))
    for i, it := range a.entries {
        a.items[i] = &AtomItem{it}
    }
}

func (a *AtomFeed) Items() []Item {
    return a.items
}

func (a *AtomFeed) ShiftedAt(n int, t time.Time) ([]Item, error) {
    return univShiftedAt(n, t, a, a.d)
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

func NewFeed(t []byte, d *DateSource) (Feed, error) {
    doc, err := gokogiri.ParseXml(t)
    if err != nil {
        return nil, err
    }
    rss, rssErr := newRssRaw(doc)
    if rssErr == nil {
        rss.makeItems()
        rss.d = d
        return Feed(rss), nil
    }
    atom, atomErr := newAtomRaw(doc)
    if atomErr == nil {
        atom.makeItems()
        atom.d = d
        return Feed(atom), nil
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
