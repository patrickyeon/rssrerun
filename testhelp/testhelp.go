package testhelp

import (
    "strconv"
    "strings"
    "time"
)

type RSS struct {
    Items []string
}

func createRSS() *RSS {
    return new(RSS)
}

func CreateAndPopulateRSS(n int, d time.Time) *RSS {
    if n < 0 {
        return nil
    }
    ret := new(RSS)
    for i := n; i >= 1; i-- {
        pubdate := d.AddDate(0, 0, 7*(i - 1)).Format(time.RFC822)
        postText := "<title>post number " + strconv.Itoa(i) + "</title>"
        postText += "<pubDate>" + pubdate + "</pubDate>"
        postText += "<guid>" + strconv.Itoa(i) + "</guid>"
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
