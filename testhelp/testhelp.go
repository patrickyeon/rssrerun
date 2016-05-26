package testhelp

import (
    "strconv"
    "strings"
    "time"
)

type TestFeed interface {
    Items() []string
    AddPost(s string)
    Text() string
    Bytes() []byte
}

type RSS struct {
    items []string
}

func CreateAndPopulateRSS(n int, d time.Time) *RSS {
    return CreateIncompleteRSS(n, d, true, true)
}

func CreateIncompleteRSS(n int, d time.Time, addPD bool, addGuid bool) *RSS {
    if n < 0 {
        return nil
    }
    ret := new(RSS)
    for i := n; i >= 1; i-- {
        pubdate := d.AddDate(0, 0, 7*(i - 1)).Format(time.RFC822)
        postText := "<title>post number " + strconv.Itoa(i) + "</title>"
        if addPD {
            postText += "<pubDate>" + pubdate + "</pubDate>"
        }
        if addGuid {
            postText += "<guid>" + strconv.Itoa(i) + "</guid>"
        }
        postText += "<link>url://foo.bar/rss/" + strconv.Itoa(i) + "</link>"
        postText += "<description>originally published " + pubdate
        postText += "</description>"
        ret.AddPost(postText)
    }
    return ret
}

func (r *RSS) AddPost(s string) {
    r.items = append(r.items, s)
}

func (r *RSS) Text() string {
    retval := "<rss version=\"2.0\"><channel><title>foo</title>\n"
    retval += "<link>http://example.com</link>\n"
    retval += "<description>Foobity foo bar.</description>\n"
    retval += "<item>" + strings.Join(r.items, "</item>\n<item>") + "</item>\n"
    retval += "</channel></rss>\n"
    return retval
}

func (r *RSS) Bytes() []byte {
    return []byte(r.Text())
}

func (r *RSS) Items() []string {
    return r.items
}


type ATOM struct {
    items []string
    updated string
}

func CreateAndPopulateATOM(n int, d time.Time) *ATOM {
    return CreateIncompleteATOM(n, d, true)
}

func CreateIncompleteATOM(n int, d time.Time, addPublished bool) *ATOM {
    if n < 0 {
        return nil
    }
    ret := new(ATOM)
    for i := n; i >= 1; i-- {
        update := d.AddDate(0, 0, 7 * (i - 1))
        pubdate := update.Format(time.RFC822)
        postText := "<title>post number " + strconv.Itoa(i) + "</title>"
        if addPublished {
            postText += "<published>" + pubdate + "</published>"
        }
        postText += "<updated>" + pubdate + "</updated>"
        postText += "<id>url://foo.bar/rss/" + strconv.Itoa(i) + "</id>"
        postText += "<summary>originally published " + pubdate
        postText += "</summary>"
        ret.AddPost(postText)
        // FIXME this needs to check for after
        if ret.updated == "" {
            ret.updated = pubdate
        }
    }
    return ret
}

func (a *ATOM) AddPost(s string) {
    a.items = append(a.items, s)
}

func (a *ATOM) Text() string {
    retval := "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"
    retval += "<feed xmlns=\"http://www.w3.org/2005/Atom\">\n"
    retval += "<title>Pat's atom feed </title>\n"
    retval += "<id>http://example.com</id>\n"
    retval += "<updated>" + a.updated + "</updated>\n"
    retval += "<entry>" + strings.Join(a.items, "</entry>\n<entry>")
    retval += "</entry>\n"
    retval += "</feed>\n"
    return retval
}

func (a *ATOM) Bytes() []byte {
    return []byte(a.Text())
}

func (a *ATOM) Items() []string {
    return a.items
}
