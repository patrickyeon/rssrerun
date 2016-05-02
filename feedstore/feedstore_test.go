package feedstore

import (
    "os"
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
    return *NewStore(TDir + "/store/")
}

func createItems(n int, start time.Time) ([]xml.Node, []string, error) {
    rss := testhelp.CreateAndPopulateRSS(n, start)
    chronItems := make([]xml.Node, n)
    for i := 0; i < n; i++ {
        it, err := gokogiri.ParseXml([]byte("<item>" + rss.Items[n - 1 - i] + "</item>"))
        if err != nil {
            return nil, nil, err
        }
        chronItems[i] = it.Root()
    }
    return chronItems, rss.Items, nil
}


func TestStoreItems(t *testing.T) {
    nIt := 15
    s := emptyStore()
    url := "test://testurl.whatever"
    if s.NumItems(url) != 0 {
        t.Fatal("empty store was not actually empty")
    }
    s.CreateIndex(url)

    items, _, err := createItems(nIt, startDate())
    if err != nil {
        t.Fatal(err)
    }

    err = s.Update(url, items)
    if err != nil {
        t.Fatal(err)
    }

    if n := s.NumItems(url); n != nIt {
        t.Fatalf("expected to store nIt items, actually reporting %d.\n", n)
    }
}

func TestStoreAndRetrieve(t *testing.T) {
    s := emptyStore()
    url := "test://testurl.whatevs"
    nItems := 25
    items, _, err := createItems(nItems, startDate())
    if err != nil {
        t.Fatal(err)
    }
    s.CreateIndex(url)
    _ = s.Update(url, items)
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
                if its[0].String() != items[i].String() {
                    t.Error(its[0].String())
                }
            }
        }
    }
}

func TestHashCollisions(t *testing.T) {
    //FIXME doesn't really exercise, no evidence that parent still works
    s := emptyStore()
    url := "test://testurl.whatevs"
    aggrUrl := "test://break.stuff"
    s.CreateIndex(url)
    aggr, _ := s.CreateIndex(aggrUrl)
    aggr.Hash = key(url)
    err := s.saveIndex(aggr)
    if err != nil {
        t.Fatalf("error saving aggressor, %s\n", err)
    }
    _, err = s.CreateIndex(url)
    if err != nil {
        t.Fatalf("couldn't create index with a collision, %s\n", err)
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
