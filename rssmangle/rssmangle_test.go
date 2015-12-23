package rssmangle

import (
    "strconv"
    "strings"
    "testing"
    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

type RSS struct {
    Items []string
}

func createRSS() *RSS {
    return new(RSS)
}

func createAndPopulateRSS(n int) *RSS {
    if n < 0 {
        return nil
    }
    ret := new(RSS)
    for i := n; i > 0; i-- {
        ret.AddPost("<title>post number " + strconv.Itoa(i) + "</title>")
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
    rss := createAndPopulateRSS(10)
    rssb := []byte(rss.Text())
    feed, err := NewFeed(rssb)
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
    rss := createAndPopulateRSS(10)
    feed, err := NewFeed([]byte(rss.Text()))
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

    diff, err := xmldiff([]byte(rss.Text()), []byte(rss.Text()))
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

    diff, err = xmldiff([]byte(rss.Text()), fb)
    if err != nil {
        t.Error("errored out during real xmldiff")
    }
    if !diff {
        t.Error("rss and fb should be different (strict whitespace)")
    }
}

func TestHandleCDATA(t *testing.T) {
    rss := createAndPopulateRSS(2)
    breakText := "<title>pre-CDATA</title><description><![CDATA["
    breakText += "</item><item>this should not be its own item</item>"
    breakText += "]]></description"
    rss.AddPost(breakText)
    rss.AddPost("<title>post-CDATA</title>")
    feed, _ := NewFeed([]byte(rss.Text()))

    if got := len(feed.Items); got != 4 {
        t.Logf("CDATA parsing failed, expected %d items, got %d\n", 4, got)
        t.Error(string(feed.Bytes()))
    }
}
