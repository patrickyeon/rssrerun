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

func TestStoreItems(t *testing.T) {
    nIt := 15
    s := emptyStore()
    url := "test://testurl.whatever"
    if s.NumItems(url) != 0 {
        t.Fatal("empty store was not actually empty")
    }

    rss := testhelp.CreateAndPopulateRSS(nIt, startDate())
    chronItems := make([]xml.Node, nIt)
    for i := 0; i < nIt; i++ {
        it, err := gokogiri.ParseXml([]byte("<item>" + rss.Items[nIt - 1 - i] + "</item>"))
        if err != nil {
            t.Fatal(err)
        }
        chronItems[i] = it.Root()
    }

    err := s.Update(url, chronItems)
    if err != nil {
        t.Fatal(err)
    }

    if n := s.NumItems(url); n != nIt {
        t.Fatalf("expected to store nIt items, actually reporting %d.\n", n)
    }
}
