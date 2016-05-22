package rssmangle

import (
    "strconv"
    "testing"
    "time"

    "rss-rerun/datesource"
    "rss-rerun/testhelp"
)

func startDate() time.Time {
    return time.Date(2015, 4, 12, 1, 0, 0, 0, time.UTC)
}

func TestHandleCDATA(t *testing.T) {
    rss := testhelp.CreateAndPopulateRSS(2, startDate())
    breakText := "<title>pre-CDATA</title><description><![CDATA["
    breakText += "</item><item>this should not be its own item</item>"
    breakText += "]]></description"
    rss.AddPost(breakText)
    rss.AddPost("<title>post-CDATA</title>")
    feed, _ := NewFeed(rss.Bytes(), nil)

    if got := len(feed.Items()); got != 4 {
        t.Logf("CDATA parsing failed, expected %d items, got %d\n", 4, got)
        t.Error(string(feed.Bytes()))
    }
}

func TestTimeShift(t *testing.T) {
    sched := []time.Weekday{time.Sunday, time.Tuesday}
    rss := testhelp.CreateAndPopulateRSS(10, startDate())
    rerun := datesource.NewDateSource(startDate().AddDate(0, 2, 0), sched)

    feed, _ := NewFeed(rss.Bytes(), rerun)
    feed.TimeShift()

    shifted, err := NewFeed(feed.Bytes(), nil)
    if err != nil {
        t.Error(err)
    }
    if got := len(shifted.Items()); got != len(feed.Items()) {
        t.Errorf("expected %d items, got %d\n", len(feed.Items()), got)
    }

    expected := datesource.NewDateSource(startDate().AddDate(0, 2, 0), sched)
    for i := (len(shifted.Items()) - 1); i >= 0; i-- {
        it := shifted.Item(i)
        pd, err := it.PubDate()
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
    rss := testhelp.CreateAndPopulateRSS(100, startDate().AddDate(-3, 0, 0))
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

    prev, err := items[0].PubDate()
    if err != nil {
        t.Fatal(err)
    }
    for i, _ := range items {
        itdate, err := items[i].PubDate()
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

func TestGuids(t *testing.T) {
    rss := testhelp.CreateAndPopulateRSS(10, startDate())
    feed, _ := NewFeed(rss.Bytes(), nil)

    for i, item := range feed.Items() {
        g, err := item.Guid()
        if err != nil {
            t.Fatalf("Item %d: %v", i, err)
        }
        if strconv.Itoa(10 - i) != g {
            // 10 - i because items count backwards
            t.Fatalf("item %d has wrong guid: %s should be %d", i, g, 10 - i)
        }
    }
}

func TestCreativeGuids(t *testing.T) {
    rss := testhelp.CreateIncompleteRSS(10, startDate(), true, false)
    feed, _ := NewFeed(rss.Bytes(), nil)

    guidSet := make(map[string]bool, 10)

    for _, item := range feed.Items() {
        g, err := item.Guid()
        if err != nil {
            t.Fatal(err)
        }
        guidSet[g] = true
    }
    if len(guidSet) != 10 {
        t.Errorf("guids not created successfully (%d/%d):", len(guidSet), 10)
        for g := range guidSet {
            t.Error(g)
        }
        t.Fail()
    }
}
