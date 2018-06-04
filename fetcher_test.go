package rssrerun

import (
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/patrickyeon/rssrerun/testhelp"
)

func TestFetchOneMemento (t *testing.T) {
    rss := testhelp.CreateAndPopulateRSS(10, mkDate(2008, 3, 4))
    serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
                                                     r *http.Request) {
        fmt.Fprintf(w, rss.Text())
    }))
    _, ts := mkServer("http://example.com", "http://tg.com/example.com",
                      []string{serv.URL}, mkDate(2018, 3, 2))
    feed, extra, err := FeedFromArchive(ts.URL)
    if err != nil {
        t.Fatal(err)
    }
    if nItems := feed.LenItems(); nItems != 10 {
        t.Errorf("Didn't get all items. Expected 10, got %d.", nItems)
    }
    if len(extra) != 0 {
        t.Errorf("extra should've been empty, but isn't.")
    }
}

func TestFetchMementosNoRedundancy (t *testing.T) {
    items := testhelp.CreateAndPopulateRSS(20, mkDate(2008, 3, 4)).Items()
    srv1 := itemServer(items[:10])
    srv2 := itemServer(items[10:])
    //  For now at least, don't care that dates from timegate don't make sense
    // vs. the dates in the feeds.
    _, ts := mkServer("http://example.com", "http://tg.com/example.com",
                      []string{srv2.URL, srv1.URL}, mkDate(2018, 3, 2))
    feed, extra, err := FeedFromArchive(ts.URL)
    if err != nil {
        t.Fatal(err)
    }
    if nItems := feed.LenItems(); nItems != 10 {
        t.Errorf("Didn't get all items. Expected 10, got %d.", nItems)
    }
    if len(extra) != 10 {
        t.Errorf("Didn't get all extra items. Expected 10, got %d.", len(extra))
    }
    for i := 0; i < feed.LenItems(); i++ {
        guidLHS, err := feed.Item(i).Guid()
        if err != nil {
            t.Error(err)
        }
        for _, item := range extra {
            guidRHS, err := item.Guid()
            if err != nil {
                t.Error(err)
            }
            if guidLHS == guidRHS {
                t.Errorf("GUID is present in more than one place: %s", guidLHS)
            }
        }
    }
}

func TestFetchMementosWithRedundancies (t *testing.T) {
    items := testhelp.CreateAndPopulateRSS(20, mkDate(2008, 3, 4)).Items()
    srv1 := itemServer(items[:14])
    srv2 := itemServer(items[10:])
    _, ts := mkServer("http://example.com", "http://tg.com/example.com",
                      []string{srv2.URL, srv1.URL}, mkDate(2018, 3, 2))
    feed, extra, err := FeedFromArchive(ts.URL)
    if err != nil {
        t.Fatal(err)
    }
    allItems := append(feed.Items(0, feed.LenItems()), extra...)
    if len(allItems) != 20 {
        t.Errorf("Didn't get all items. Expected 20, got %d.", len(allItems))
    }
}

func stringServer(s string) *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
                                                    r *http.Request) {
        fmt.Fprintf(w, s)
    }))
}

func itemServer(items []string) *httptest.Server {
    retrss := new(testhelp.RSS)
    for _, item := range(items) {
        retrss.AddPost(item)
    }
    return stringServer(retrss.Text())
}
