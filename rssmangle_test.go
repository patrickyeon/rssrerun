package rssrerun

import (
    "strconv"
    "testing"
    "time"

    "github.com/patrickyeon/rssrerun/testhelp"
    "github.com/jbowtie/gokogiri"
)

func TestCreateRssItem(t *testing.T) {
    rsstxt := "<item><title>Actual rss item</title>"
    rsstxt += "<pubDate>" + testhelp.StartDate().Format(time.RFC822) + "</pubDate>"
    rsstxt += "<guid>32</guid><link>foo://bar.baz/</link>"
    rsstxt += "<description>bippity boppity boo</description></item>"
    it, err := MkItem([]byte(rsstxt))
    if err != nil {
        t.Fatal(err)
    }
    g, err := it.Guid()
    if err != nil {
        switch it := it.(type) {
        default:
            t.Fatal("foo")
        case *RssItem:
            a, _ := it.src.Search("guid")
            t.Error(strconv.Itoa(len(a)))
            t.Fatalf("guid should've been 32, but it's %s", g)
        }
        t.Fatal(err)
    }
    if g != "32" {
        switch it := it.(type) {
        default:
            t.Fatal("foo")
        case *RssItem:
            a, _ := it.src.Search("guid")
            t.Error(strconv.Itoa(len(a)))
            t.Fatalf("guid should've been 32, but it's %s", g)
        }
    }
}

func TestRssHandleCDATA(t *testing.T) {
    rss := testhelp.CreateAndPopulateRSS(2, testhelp.StartDate())
    breakText := "<item><title>pre-CDATA</title><description><![CDATA["
    breakText += "</item><item>this should not be its own item</item>"
    breakText += "]]></description></item>"
    rss.AddPost(breakText)
    rss.AddPost("<item><title>post-CDATA</title></item>")
    feed, _ := NewFeed(rss.Bytes(), nil)

    if got := feed.LenItems(); got != 4 {
        t.Logf("CDATA parsing failed, expected %d items, got %d", 4, got)
        t.Error(string(feed.BytesWithItems(feed.Items(0, feed.LenItems()))))
    }
}

func TestAtomHandleCDATA(t *testing.T) {
    atom := testhelp.CreateAndPopulateATOM(2, testhelp.StartDate())
    breakText := "<id>foo://bar/bazprecdat</id><content><![CDATA["
    breakText += "</entry><entry>this should not be its own entry</entry>"
    breakText += "]]></content><title>pre CDATA</title>"
    atom.AddPost(breakText)
    atom.AddPost("<id>foo://bar/bazpostcdat</id><title>post CDATA</title>")
    feed, _ := NewFeed(atom.Bytes(), nil)

    if got := feed.LenItems(); got != 4 {
        t.Logf("CDATA parsing failed, expected %d items, got %d", 4, got)
        t.Error(string(feed.BytesWithItems(feed.Items(0, feed.LenItems()))))
    }
}

func TestRssTimeShift(t *testing.T) {
    testTimeShift(t, testhelp.CreateAndPopulateRSS(10, testhelp.StartDate()),
                  NewFeed)
}

func TestAtomTimeShift(t *testing.T) {
    testTimeShift(t, testhelp.CreateAndPopulateATOM(10, testhelp.StartDate()),
                  NewFeed)
}

func testTimeShift(t *testing.T, tf testhelp.TestFeed,
                   newFeed func([]byte, *DateSource)(Feed, error)) {
    sched := []time.Weekday{time.Sunday, time.Tuesday}
    rerun := NewDateSource(testhelp.StartDate().AddDate(0, 2, 0), sched)
    expected := NewDateSource(testhelp.StartDate().AddDate(0, 2, 0), sched)
    feed, err := newFeed(tf.Bytes(), rerun)
    if err != nil {
        t.Fatal(err)
    }

    if got := feed.LenItems(); got != len(tf.Items()) {
        t.Fatalf("expected %d items, got %d", len(tf.Items()), got)
    }

    shifted, err := feed.ShiftedAt(100, testhelp.StartDate().AddDate(0, 0, 100))
    if err != nil {
        t.Error(err)
    }
    if got := len(shifted); got != feed.LenItems() {
        t.Fatalf("expected %d items, got %d", feed.LenItems(), got)
    }

    for i := (len(shifted) - 1); i >= 0; i-- {
        it := shifted[i]
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

func TestRssLatestFive(t *testing.T) {
    rss := testhelp.CreateAndPopulateRSS(100, testhelp.StartDate().AddDate(-3, 0, 0))
    testLatestFive(t, rss)
}

func TestAtomLatestFive(t *testing.T) {
    atom := testhelp.CreateAndPopulateATOM(100, testhelp.StartDate().AddDate(-3, 0, 0))
    testLatestFive(t, atom)
}

func testLatestFive(t *testing.T, tf testhelp.TestFeed) {
    sched := []time.Weekday{time.Sunday, time.Tuesday}
    rerun := NewDateSource(testhelp.StartDate(), sched)
    feed, _ := NewFeed(tf.Bytes(), rerun)
    now := testhelp.StartDate().AddDate(0, 4, 0)

    items, err := feed.ShiftedAt(5, now)
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

    // TODO I don't like reaching in to the object like this
    var future time.Time
    if rssfeed, ok := feed.(*RssFeed); ok {
        future, err = rssfeed.d.NextDate()
    } else if atomfeed, ok := feed.(*AtomFeed); ok {
        future, err = atomfeed.d.NextDate()
    } else {
        t.Fatal("feed object that is neither RSS nor Atom")
    }
    if err != nil {
        t.Fatal(err)
    }
    if future.Before(now) {
        for _, i := range items {
            t.Error(i.String())
        }
        t.Fatal("still item(s) available before 'now'")
    }
}

func TestRssGuids(t *testing.T) {
    testGuids(t, testhelp.CreateAndPopulateRSS(10, testhelp.StartDate()))
}

func TestAtomGuids(t *testing.T) {
    testGuids(t, testhelp.CreateAndPopulateATOM(10, testhelp.StartDate()))
}

func testGuids(t *testing.T, tf testhelp.TestFeed) {
    feed, err := NewFeed(tf.Bytes(), nil)

    if err != nil {
        t.Fatal(err)
    }
    if nitms := feed.LenItems(); nitms != 10 {
        t.Error(feed.Items(0, nitms))
        t.Fatalf("expected 10 items, got %d", nitms)
    }

    for i, item := range feed.Items(0, feed.LenItems()) {
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

func TestRssCreativeGuids(t *testing.T) {
    testCreativeGuids(t, testhelp.CreateIncompleteRSS(10, testhelp.StartDate(),
                                                      true, false))
}

// we shouldn't need creative guids for atom

func testCreativeGuids(t *testing.T, tf testhelp.TestFeed) {
    feed, _ := NewFeed(tf.Bytes(), nil)
    guidSet := make(map[string]bool, 10)

    for _, item := range feed.Items(0, feed.LenItems()) {
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

func TestRssFrontMatter(t *testing.T) {
    rss := testhelp.CreateAndPopulateRSS(12, testhelp.StartDate())
    feed, _ := NewFeed(rss.Bytes(), nil)

    wrapper, err := gokogiri.ParseXml(feed.Wrapper())
    if err != nil {
        t.Fatal(err)
    }

    its, err := wrapper.Root().Search("channel/item")
    if err != nil {
        t.Fatal(err)
    }
    if len(its) != 1 {
        t.Fatalf("should have 1 and only 1 <item> tag. found %d", len(its))
    }
}

func TestAtomFrontMatter(t *testing.T) {
    atom := testhelp.CreateAndPopulateATOM(12, testhelp.StartDate())
    feed, _ := NewFeed(atom.Bytes(), nil)

    wrapper, err := gokogiri.ParseXml(feed.Wrapper())
    if err != nil {
        t.Fatal(err)
    }

    its, err := wrapper.Root().Search("//*[local-name()='entry']")
    if err != nil {
        t.Fatal(err)
    }
    if len(its) != 1 {
        t.Fatalf("should have 1 and only 1 <entry> tag. found %d", len(its))
    }
}
