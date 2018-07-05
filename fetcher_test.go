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
    feed, err := FeedFromArchive(ts.URL)
    checkItemCount(feed, err, 10, t)
}

func TestFetchMementosNoRedundancy (t *testing.T) {
    items := testhelp.CreateAndPopulateRSS(20, mkDate(2008, 3, 4)).Items()
    srv1 := itemServer(items[:10])
    srv2 := itemServer(items[10:])
    //  For now at least, don't care that dates from timegate don't make sense
    // vs. the dates in the feeds.
    _, ts := mkServer("http://example.com", "http://tg.com/example.com",
                      []string{srv1.URL, srv2.URL}, mkDate(2018, 3, 2))
    feed, err := FeedFromArchive(ts.URL)
    checkItemCount(feed, err, 20, t)
}

func TestFetchMementosWithRedundancies (t *testing.T) {
    items := testhelp.CreateAndPopulateRSS(20, mkDate(2008, 3, 4)).Items()
    srv1 := itemServer(items[:14])
    srv2 := itemServer(items[10:])
    _, ts := mkServer("http://example.com", "http://tg.com/example.com",
                      []string{srv1.URL, srv2.URL}, mkDate(2018, 3, 2))
    feed, err := FeedFromArchive(ts.URL)
    checkItemCount(feed, err, 20, t)
}

func TestFetchMementosAllRedundant (t *testing.T) {
    items := testhelp.CreateAndPopulateRSS(10, mkDate(2008, 3, 4)).Items()
    srv1 := itemServer(items)
    srv2 := itemServer(items)
    _, ts := mkServer("http://example.com", "http://tg.com/example.com",
                      []string{srv1.URL, srv2.URL}, mkDate(2018, 3, 2))
    feed, err := FeedFromArchive(ts.URL)
    checkItemCount(feed, err, 10, t)
}

func TestCanSkipMemento (t *testing.T) {
    items := testhelp.CreateAndPopulateRSS(15, mkDate(2008, 3, 4)).Items()
    srv1 := itemServer(items[:10])
    srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
                                                     r *http.Request) {
                _, _ = w, r
                t.Fatal("Extra fetch. You should be smarter than that.")
            }))
    srv3 := itemServer(items[7:])
    _, ts := mkServer("http://example.com", "http://tg.com/example.com",
                      []string{srv1.URL, srv2.URL, srv3.URL},
                      mkDate(2018, 3, 2))
    feed, err := FeedFromArchive(ts.URL)
    checkItemCount(feed, err, 15, t)
}

func TestFetchPerfectSplit (t *testing.T) {
    // if we're assuming an overlap somewhere, this should expose it
    // TODO the bug this originally exposed caused an infinite loop. I would
    //      like a better way to detect that.
    items := testhelp.CreateAndPopulateRSS(15, mkDate(2008, 3, 4)).Items()
    srv1 := itemServer(items[:5])
    srv2 := itemServer(items[5:10])
    srv3 := itemServer(items[10:])
    _, ts := mkServer("http://example.com", "http://tg.com/example.com",
                      []string{srv1.URL, srv2.URL, srv3.URL},
                      mkDate(2018, 3, 2))
    feed, err := FeedFromArchive(ts.URL)
    checkItemCount(feed, err, 15, t)
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

func checkAllUnique(items []Item, t *testing.T) {
    seenGuids := make(map[string]bool)
    for _, item := range(items) {
        guid, err := item.Guid()
        if err != nil {
            t.Error(err)
            continue
        }
        if _, seen := seenGuids[guid]; seen {
            t.Errorf("item duplicated: %s.", guid)
            continue
        }
        seenGuids[guid] = true
    }
}

func checkItemCount(feed Feed, err error, count int, t *testing.T) {
    if err != nil {
        t.Fatal(err)
    }
    if nItems := len(feed.allItems()); nItems != count {
        t.Errorf("Got incorrect number of items. Expected %d, got %d.",
                 count, nItems)
    }
    checkAllUnique(feed.allItems(), t)
}
