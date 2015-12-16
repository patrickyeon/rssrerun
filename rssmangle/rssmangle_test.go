package rssmangle

import (
    "strconv"
    "strings"
    "testing"
)

type RSS struct {
    Items []string
}

func createRSS() *RSS {
    return new(RSS)
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

func TestIngest(t *testing.T) {
    rss := createRSS()
    for i := 0; i < 10; i++ {
        rss.AddPost("<title>post number " + strconv.Itoa(i) + "</title>")
    }
    feed := NewFeed(strings.NewReader(rss.Text()))
    if len(feed.Items) != 10 {
        t.Errorf("expected %d items, got %d", 10, len(feed.Items))
    }
}
