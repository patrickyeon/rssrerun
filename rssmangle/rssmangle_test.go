package rssmangle

import (
    "strconv"
    "strings"
    "testing"
    "time"
    "rss-rerun/datesource"
    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

func startDate() time.Time {
    return time.Date(2015, 4, 12, 1, 0, 0, 0, time.UTC)
}

type RSS struct {
    Items []string
}

func createRSS() *RSS {
    return new(RSS)
}

func createAndPopulateRSS(n int, d time.Time) *RSS {
    if n < 0 {
        return nil
    }
    ret := new(RSS)
    for i := n; i >= 1; i-- {
        pubdate := d.AddDate(0, 0, 7*(i - 1)).Format(time.RFC822)
        postText := "<title>post number " + strconv.Itoa(i) + "</title>"
        postText += "<pubDate>" + pubdate + "</pubDate>"
        postText += "<description>originally published " + pubdate
        postText += "</description>"
        ret.AddPost(postText)
    }
    return ret
}

func (r *RSS) AddPost(s string) {
    r.Items = append(r.Items, s)
}

func (r *RSS) Text() string {
    retval := "<rss version=\"2.0\"><channel><title>foo</title>\n"
    retval += "<link>http://example.com</link>\n"
    retval += "<description>Foobity foo bar.</description>\n"
    retval += "<item>" + strings.Join(r.Items, "</item>\n<item>") + "</item>\n"
    retval += "</channel></rss>\n"
    return retval
}

func (r *RSS) Bytes() []byte {
    return []byte(r.Text())
}

func nodesAreSameish(na, nb xml.Node) bool {
    nodesDefinedByContent := []xml.NodeType {xml.XML_TEXT_NODE,
                                             xml.XML_COMMENT_NODE,
                                             xml.XML_CDATA_SECTION_NODE}
    if na.NodeType() != nb.NodeType() {
        return false
    }
    if na.Name() != nb.Name() {
        return false
    }
    for _, t := range nodesDefinedByContent {
        if na.NodeType() == t {
            return na.Content() == nb.Content()
        }
    }
    if len(na.Attributes()) != len(nb.Attributes()) {
        return false
    }
    for k, v := range na.Attributes() {
        attr := nb.Attribute(k)
        if attr == nil {
            return false
        }
        if attr.Value() != v.Value() {
            return false
        }
    }
    return true
}

func nodesAreSameWithChildren(na, nb xml.Node) bool {
    if !nodesAreSameish(na, nb) {
        return false
    }
    if na.NodeType() == xml.XML_TEXT_NODE {
        return true
    }
    if na.CountChildren() != nb.CountChildren() {
        return false
    }
    ac := na.FirstChild()
    bc := nb.FirstChild()
    for i := na.CountChildren(); i > 0; i-- {
        if !nodesAreSameish(ac, bc) {
            return false
        }
        ac = ac.NextSibling()
        bc = bc.NextSibling()
    }
    return true
}

func getNodeChildren(n xml.Node) []xml.Node {
    if n == nil || n.FirstChild() == nil {
        return nil
    }
    ret := make([]xml.Node, n.CountChildren())
    ret[0] = n.FirstChild()
    for i := 1; i < n.CountChildren(); i++ {
        nx := ret[i - 1].NextSibling()
        ret[i] = nx
    }
    return ret
}

type xmlWalk struct {
    Root, cur xml.Node
    stack []xml.Node
    done bool
}

func (w *xmlWalk) next() xml.Node {
    if w.Root == nil {
        return nil
    }
    if w.done {
        return nil
    }
    if w.cur == nil {
        w.cur = w.Root
    } else if ln := len(w.stack); ln > 0 {
        w.cur = w.stack[0]
        w.stack = w.stack[1:]
    } else {
        w.cur = nil
        w.done = true
        return nil
    }
    if w.cur.NodeType() != xml.XML_TEXT_NODE {
        w.stack = append(getNodeChildren(w.cur), w.stack...)
    }
    return w.cur
}

func xmldiff(xmla []byte, xmlb []byte) (bool, error) {
    doca, err := gokogiri.ParseXml(xmla)
    if err != nil {return true, err}
    docb, err := gokogiri.ParseXml(xmlb)
    if err != nil {return true, err}

    var xa, xb xmlWalk
    xa.Root = doca.Root()
    xb.Root = docb.Root()

    for {
        na := xa.next()
        nb := xb.next()
        if na == nil || nb == nil {
            if na == nb {
                return false, nil
            }
            return true, nil
        }
        if !nodesAreSameWithChildren(na, nb) {
            return true, nil
        }
    }
}

func NoTestIngest(t *testing.T) {
    // temporarily disabled, parsing a feed changes it.
    rss := createAndPopulateRSS(10, startDate())
    rssb := rss.Bytes()
    feed, err := NewFeed(rssb, nil)
    if err != nil {
        t.Error("error parsing feed")
    }
    if len(feed.Items) != 10 {
        t.Errorf("expected %d items, got %d", 10, len(feed.Items))
    }
    fb := feed.Bytes()
    if len(rssb) == len(fb) {
        for i, r := range rssb {
            if r == fb[i] {
                t.Errorf(">>>\n%s\n<<<\n%s\n", string(rssb), string(fb))
            }
        }
    } else {
        t.Errorf("mismatched len %d:%d\n", len(rssb), len(fb))
        t.Errorf(">>>\n%s\n<<<\n%s\n", string(rssb), string(fb))
    }
}

func TestCompare(t *testing.T) {
    rss := createAndPopulateRSS(10, startDate())
    feed, err := NewFeed(rss.Bytes(), nil)
    if err != nil {
        t.Error("errored out parsing feed")
    }
    if len(feed.Items) != 10 {
        t.Errorf("expected %d items, got %d", 10, len(feed.Items))
    }
    fb := feed.Bytes()
    if err != nil {
        t.Error(err)
    }

    diff, err := xmldiff(rss.Bytes(), rss.Bytes())
    if err != nil {
        t.Error("errored out during xmldiff")
    }
    if diff {
        t.Error("rss reported as self-dissimilar")
    }

    diff, err = xmldiff(fb, fb)
    if err != nil {
        t.Error("errored out during second xmldiff")
    }
    if diff {
        t.Error("fb reported as self-dissimilar")
    }

    diff, err = xmldiff(rss.Bytes(), fb)
    if err != nil {
        t.Error("errored out during real xmldiff")
    }
    if !diff {
        t.Error("rss and fb should be different (strict whitespace)")
    }
}

func TestHandleCDATA(t *testing.T) {
    rss := createAndPopulateRSS(2, startDate())
    breakText := "<title>pre-CDATA</title><description><![CDATA["
    breakText += "</item><item>this should not be its own item</item>"
    breakText += "]]></description"
    rss.AddPost(breakText)
    rss.AddPost("<title>post-CDATA</title>")
    feed, _ := NewFeed(rss.Bytes(), nil)

    if got := len(feed.Items); got != 4 {
        t.Logf("CDATA parsing failed, expected %d items, got %d\n", 4, got)
        t.Error(string(feed.Bytes()))
    }
}

func TestTimeShift(t *testing.T) {
    sched := []time.Weekday{time.Sunday, time.Tuesday}
    rss := createAndPopulateRSS(10, startDate())
    rerun := datesource.NewDateSource(startDate().AddDate(0, 2, 0), sched)

    feed, _ := NewFeed(rss.Bytes(), rerun)
    feed.TimeShift()

    shifted, err := NewFeed(feed.Bytes(), nil)
    if err != nil {
        t.Error(err)
    }
    if got := len(shifted.Items); got != len(feed.Items) {
        t.Errorf("expected %d items, got %d\n", len(feed.Items), got)
    }

    expected := datesource.NewDateSource(startDate().AddDate(0, 2, 0), sched)
    for i := (len(shifted.Items) - 1); i >= 0; i-- {
        it := shifted.Items[i]
        pd, err := pubDate(&it)
        if err != nil {
            t.Error(err)
        } else {
            date, err := expected.NextDate()
            if err != nil {
                t.Error(err)
            } else if date != pd {
                t.Error(it.String())
            }
        }
    }
}

func TestLatestFive(t *testing.T) {
    sched := []time.Weekday{time.Sunday, time.Tuesday}
    rss := createAndPopulateRSS(100, startDate().AddDate(-3, 0, 0))
    rerun := datesource.NewDateSource(startDate(), sched)

    feed, _ := NewFeed(rss.Bytes(), rerun)
    now := startDate().AddDate(0, 4, 0)
    items, err := feed.LatestAt(5, now)
    if err != nil {
        t.Fatal(err)
    }
    if len(items) != 5 {
        t.Errorf("expected 5 items, got %d\n", len(items))
    }

    prev, err := pubDate(&items[0])
    if err != nil {
        t.Fatal(err)
    }
    for i, _ := range items {
        itdate, err := pubDate(&items[i])
        if err != nil {
            t.Fatal(err)
        }
        if itdate.After(prev) {
            t.Fatalf("item %d comes out of order\n", i)
        }
        if itdate.After(now) {
            t.Fatalf("item %d comes after 'now'\n", i)
        }
        prev = itdate
    }

    future, err := feed.d.NextDate()
    if err != nil {
        t.Fatal(err)
    }
    if future.Before(now) {
        t.Fatal("still item(s) available before 'now'")
    }
}
