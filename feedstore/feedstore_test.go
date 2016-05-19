package feedstore

import (
    "os"
    "strings"
    "testing"
    "time"

    "rss-rerun/testhelp"

    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

const (
    TDir = "testdat"
)

func startDate() time.Time {
    return time.Date(2015, 4, 12, 1, 0, 0, 0, time.UTC)
}

func emptyStore() Store {
    _ = os.RemoveAll(TDir + "/store")
    _ = os.Mkdir(TDir + "/store", os.ModeDir | os.ModePerm)
    ret := NewStore(TDir + "/store/")
    ret.canon = func (url string) (string, error) {return url, nil}
    return *ret
}

func createItems(n int, start time.Time) ([][]byte, []string, error) {
    rss := testhelp.CreateAndPopulateRSS(n, start)
    itemBytes := make([][]byte, len(rss.Items))
    for i, item := range rss.Items {
        itemBytes[i] = append(append([]byte("<item>"), []byte(item)...), []byte("</item>")...)
    }
    return itemBytes, rss.Items, nil
}

func parsedItems(n int, rss *testhelp.RSS) ([]xml.Node, error) {
    retItems := make([]xml.Node, n)
    for i := 0; i < n; i++ {
        it, err := gokogiri.ParseXml([]byte("<item>" + rss.Items[n - 1 - i] + "</item>"))
        if err != nil {
            return nil, err
        }
        retItems[i] = it.Root()
    }
    return retItems, nil
}

func TestStoreItems(t *testing.T) {
    nIt := 15
    s := emptyStore()
    url := "test://testurl.whatever"
    if s.NumItems(url) != 0 {
        t.Fatal("empty store was not actually empty")
    }
    _, err := s.CreateIndex(url)
    if err != nil {
        t.Fatal(err)
    }

    items, _, err := createItems(nIt, startDate())
    if err != nil {
        t.Fatal(err)
    }

    err = s.Update(url, items)
    if err != nil {
        t.Error(string(items[0]))
        t.Fatal(err)
    }

    if n := s.NumItems(url); n != nIt {
        t.Fatalf("expected to store nIt items, actually reporting %d.\n", n)
    }
}

func gimmeStore() (Store, string, [][]byte) {
    s := emptyStore()
    url := "test://testurl.whatevs"
    nItems := 25
    items, _, _ := createItems(nItems, startDate())
    s.CreateIndex(url)
    _ = s.Update(url, items)

    return s, url, items
}

func sameish(a []byte, b []byte) bool {
    // FIXME code should be able to return exact strings, not rely on sameish
    return strings.TrimSpace(string(a)) == strings.TrimSpace(string(b))
}

func TestStoreAndRetrieve(t *testing.T) {
    s, url, items := gimmeStore()
    nItems := len(items)
    for start := 0; start < (nItems - 5); start++ {
        for end := start + 5; end < nItems; end++ {
            for i := 0; i < 5; i++ {
                t.Log(items[i])
                its, err := s.Get(url, i, i + 1)
                if err != nil {
                    t.Fatal(err)
                }
                if len(its) != 1 {
                    t.Fatalf("Expected an item, actually got %d.\n", len(its))
                }
                if !sameish(its[0], items[i]) {
                    t.Error(string(its[0]))
                }
            }
        }
    }
}

func TestStoreAndRetrieveMany(t *testing.T) {
    s, url, items := gimmeStore()
    nItems := len(items)
    for i := 1; i < nItems; i++ {
        its, err := s.Get(url, 0, i)
        if err != nil {
            t.Fatal(err)
        }
        if len(its) != i {
            t.Fatalf("store and retrieve failed at i=%d", i)
        }
    }
}

func TestHashCollisions(t *testing.T) {
    s := emptyStore()
    s.key = func (string) string { return "hashed" }
    url := "test://testurl.whatevs"
    aggrUrl := "test://break.stuff"
    s.CreateIndex(url)
    _, err := s.CreateIndex(aggrUrl)
    if err != nil {
        t.Fatalf("error creating aggressor, %s\n", err)
    }

    vicItems, _, _ := createItems(3, startDate())
    aggrItems, _, _ := createItems(5, startDate())
    err = s.Update(aggrUrl, aggrItems)
    if err != nil {
        t.Fatal(err)
    }
    err = s.Update(url, vicItems)
    if err != nil {
        t.Fatal(err)
    }

    vicCount := s.NumItems(url)
    aggrCount := s.NumItems(aggrUrl)
    if vicCount != 3 || aggrCount != 5 {
        t.Fatalf("expected (3, 5) items, got (%d, %d).\n", vicCount, aggrCount)
    }
}

func TestUpdateFile(t *testing.T) {
    s := emptyStore()
    url := "test://testurl.whatevs"
    nItems := 30
    items, _, _ := createItems(nItems, startDate())
    s.CreateIndex(url)
    _ = s.Update(url, items[:22])
    err := s.Update(url, items[:25])
    if err != nil {
        t.Fatal(err)
    }
    err = s.Update(url, items)
    if err != nil {
        t.Fatal(err)
    }
    its, err := s.Get(url, 0, nItems)
    if err != nil {
        t.Fatal(err)
    }
    if len(its) != nItems {
        t.Fatalf("Expected %d items, got %d", nItems, len(its))
    }
    for i, it := range its {
        if !sameish(it, items[i]) {
            t.Fatal("::" + string(it) + "::\n--" + string(items[i]) + "--")
        }
    }
    its, err = s.Get(url, 3, nItems)
    if err != nil {
        t.Fatal(err)
    }
    if len(its) != (nItems - 3) {
        t.Fatalf("Expected %d items, got %d", nItems - 3, len(its))
    }
    for i, it := range its {
        if !sameish(it, items[i + 3]) {
            t.Fatal(string(it))
        }
    }
}

func TestMetaVals(t *testing.T) {
    s, u, _ := gimmeStore()
    val, err := s.GetInfo(u, "foo")
    if err != nil {
        t.Fatal(err)
    }
    if val != "" {
        t.Fatal("expected empty response, got: %s", val)
    }

    if err = s.SetInfo(u, "bar", "baz"); err != nil {
        t.Fatal(err)
    }
    val, err = s.GetInfo(u, "foo")
    if err != nil {
        t.Fatal(err)
    }
    if val != "" {
        t.Fatal("expected empty response, got: %s", val)
    }
    val, err = s.GetInfo(u, "bar")
    if err != nil {
        t.Fatal(err)
    }
    if val != "baz" {
        t.Fatal("expected baz, got: %s", val)
    }
}

func TestNoGuid(t *testing.T) {
    s := emptyStore()
    url := "test://testurl.whatnot"
    nItems := 12
    rss := testhelp.CreateIncompleteRSS(nItems, startDate(), true, false)
    items := make([][]byte, len(rss.Items))
    for i, item := range rss.Items {
        items[i] = append(append([]byte("<item>"), []byte(item)...), []byte("</item>")...)
    }

    s.CreateIndex(url)
    err := s.Update(url, items)
    if err != nil {
        t.Fatal(err)
    }
}
