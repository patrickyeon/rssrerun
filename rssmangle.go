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

//  Pretty much what it says on the tin: the XML representation of a feed.
type Feed interface {
    //  Return the document with only one item/entry tag. It is an empty tag and
    // used as a placeholder to be populated with items/entries later.
    Wrapper() []byte
    // Return the doc with the placeholder replaced with `items`
    BytesWithItems(items []Item) []byte
    // Accessor for the `Item`s parsed from the doc.
    // `Item`s are stored in the order listed in the doc. I guess this doesn't
    // necessarily need to be chronological, but we should hope it's most
    // recent first
    Items(start int, end int) []Item
    LenItems() int
    Item(idx int) Item
    //  Return the most recent `n` `item`s, that would be replayed before `t`.
    // Errors on no items. (TODO could probably just return a `nil` array)
    ShiftedAt(n int, t time.Time) ([]Item, error)

    // Some private methods to make my life easier
    appendItems(items []Item)
    allItems() []Item
}

//  The method to shift a feed is the same whether RSS or Atom, so the work is
// abstracted out to this function. The different implementations of feeds just
// call here.
func univShiftedAt(n int, t time.Time, f Feed, d *DateSource) ([]Item, error) {
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
    nItems := f.LenItems()
    if nskip > nItems {
        return nil, errors.New("too old")
    }
    d.lastDate = d.StartDate.AddDate(0, 0, -1)
    d.SkipForward(nskip)

    nret := n
    if nret + nskip > nItems {
        // we were asked for more items than are left after skipping ahead. The
        // only time I see this happening is if `nskip == 0`, so I'm not sure
        // why `nskip` is involved here. I would guess I hit an edge case at
        // some point?
        nret = nItems - nskip
    }
    ret := make([]Item, nret)
    // TODO should I be making copies of `Item`s here? It seems weird to change
    // their pubDates without making a copy.
    for i := 0; i < nret; i++ {
        // last -1 needed because its[len(its) - x] is (x - 1)'th from the back
        ret[nret - i - 1] = f.Item(nItems - (nskip + i) - 1)
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

func (f *RssFeed) Items(start, end int) []Item {
    return f.items[start:end]
}

func (f *RssFeed) Item(idx int) Item {
    return f.items[idx]
}

func (f *RssFeed) LenItems() int {
    return len(f.items)
}

func (f *RssFeed) allItems() []Item {
    return f.items
}

func (f *RssFeed) appendItems(items []Item) {
    f.items = append(f.items, items...)
}

// TODO error on already existing here?
func (f *RssFeed) makeItems() {
    f.items = make([]Item, len(f.itemNodes))
    for i, it := range f.itemNodes {
        f.items[i] = &RssItem{it}
    }
}

//  Given an `xml.Document`, try to parse out an RSS feed and pull out the
// `item` tags.
func newRssRaw(doc xml.Document) (*RssFeed, error) {
    channels, err := doc.Root().Search("channel")
    if err != nil {
        return nil, err
    }
    if len(channels) == 0 {
        return nil, errors.New("No <channel> tag for RSS feed")
    }
    if len(channels) > 1 {
        // Spec doesn't allow this. No reason to believe it doesn't exist though
        return nil, errors.New("Too many <channel> tags for RSS feed")
    }

    f := new(RssFeed)
    f.root = doc.Root()
    f.itemNodes, err = doc.Root().Search("channel/item")
    if err != nil {
        return nil, err
    }
    if len(f.itemNodes) > 0 {
        // insert a placeholder
        f.itemNodes[0].InsertBefore(doc.CreateElementNode("item"))
    }
    // remove all the `item`s. Don't worry, we saved them in `f.itemNodes`.
    for _, item := range f.itemNodes {
        item.Unlink()
    }
    return f, nil
}

func (f *RssFeed) Wrapper() []byte {
    return f.root.ToBuffer(nil)
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

func (a *AtomFeed) Items(start, end int) []Item {
    return a.items[start:end]
}

func (a *AtomFeed) Item(idx int) Item {
    return a.items[idx]
}

func (a *AtomFeed) LenItems() int {
    return len(a.items)
}

func (a *AtomFeed) allItems() []Item {
    return a.items
}

func (a *AtomFeed) appendItems(items []Item) {
    a.items = append(a.items, items...)
}

func (a *AtomFeed) ShiftedAt(n int, t time.Time) ([]Item, error) {
    return univShiftedAt(n, t, a, a.d)
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

// Try to extract a Atom feed from an `xml.Document`
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
    // as in the RSS case, create a placeholder and remove all the `entry` tags
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

// Make a best guess at parsing a document as an RSS or Atom feed.
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

// Here is where the magic of re-populating a feed from the placeholder happens
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
    // prepare a copy to return
    retval := parent.ToBuffer(nil)
    // go back to the original feed, with place holder
    if err = children[0].AddPreviousSibling(placeholder); err != nil {
        return nil
    }
    for _, child := range children {
        child.Unlink()
    }
    return retval
}
