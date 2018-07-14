package rssrerun

import (
    "errors"
    "io/ioutil"
    "net/http"
    neturl "net/url"
    "strconv"
    "strings"
    "time"

    "github.com/jbowtie/gokogiri"
)

type FeedFunc func(string) (Feed, error)

func SelectFeedFetcher(url string) (FeedFunc, error) {
    urlmap := map[string]FeedFunc {
        // at least up to 285 episodes, libsyn just returns the whole history
        ".libsyn.com" : FeedFromUrl,
        ".libsynpro.com" : FeedFromUrl,
        "npr.org" : FeedFromNPR,
    }
    genmap := map[string]FeedFunc {
        "Site-Server v6." : FeedFromSquareSpace,
        "Libsyn WebEngine" : FeedFromUrl,
        "NPR API RSS Generator" : FeedFromNPR,
    }

    parsedUrl, err := neturl.Parse(url)
    if err != nil {
        return nil, err
    }
    hostname := parsedUrl.Hostname()
    for stub, fn := range urlmap {
        if strings.HasSuffix(hostname, stub) {
            return fn, nil
        }
    }

    resp, err := bytesFromUrl(url)
    if err != nil {
        return nil, err
    }
    doc, err := gokogiri.ParseXml(resp)
    if err != nil {
        return nil, err
    }
    gen, err := doc.Root().Search("channel/generator")
    if err != nil {
        return nil, err
    }
    if len(gen) > 0 {
        generator := gen[0].Content()
        for stub, fn := range genmap {
            if strings.HasPrefix(generator, stub) {
                return fn, nil
            }
        }
    }
    return FeedFromWayback, nil
}


func bytesFromUrl(url string) ([]byte, error) {
    retval, _, err := bytesFromUrlWithDelay(url, 0)
    return retval, err
}


func bytesFromUrlWithDelay(url string, delay int64) ([]byte, int64, error) {
    // TODO: ugggh, this is totally not the place to do this.
    if strings.HasPrefix(url, "https://web.archive.org") {
        a := strings.Split(url, "/http")
        url = a[0] + "if_/http" + a[1]
    }

    for delay < 130 {
        // arbitrarily, not backing off more than 130 sec
        time.Sleep(time.Duration(delay) * time.Second)
        resp, err := http.Get(url)
        if err != nil {
            return nil, -1, err
        }
        if resp.StatusCode == 200 {
            dat, err := ioutil.ReadAll(resp.Body)
            if err != nil {
                return nil, -1, err
            }
            return dat, delay, nil
        } else if resp.StatusCode == 429 {
            // back off like a chump
            if delay == 0 {
                delay = 1
            }
            delay *= 2
            continue
        } else {
            return nil, -1, errors.New(resp.Status)
        }
    }
    return nil, -1, errors.New("delay with backoff pushed beyond 130s. " + url)
}


func FeedFromUrl(url string) (Feed, error) {
    resp, err := bytesFromUrl(url)
    if err != nil {
        return nil, err
    }
    return NewFeed(resp, nil)
}


func iterThroughFeed(url string, fNext func(Feed, string)(string, error)) (Feed, error) {
    // TODO look through this again.
    retFeed, err := FeedFromUrl(url)
    if err != nil {
        return nil, err
    }
    if retFeed.LenItems() == 0 {
        return retFeed, nil
    }
    feed := retFeed
    for true {
        url, err := fNext(feed, url)
        if err != nil {
            return nil, err
        }
        moreFeed, err := FeedFromUrl(url)
        if err != nil {
            return nil, err
        }
        moreItems := moreFeed.allItems()
        if len(moreItems) == 0 {
            break
        }
        earliestGuid, err := retFeed.Item(0).Guid()
        if err != nil {
            return nil, err
        }
        for i := 0; i < len(moreItems); i++ {
            guid, err := moreItems[i].Guid()
            if err != nil {
                return nil, err
            }
            if guid == earliestGuid {
                if i == len(moreItems) - 1 {
                    //  seems unlikely, but with my luck it's basically
                    // guaranteed that we'll have some point that only returns
                    // items we've already seen
                    moreItems = nil
                } else {
                    moreItems = moreItems[i + 1 :]
                }
                break
            }
        }
        if len(moreItems) == 0 {
            break
        }
        retFeed.appendItems(moreItems)
        feed = moreFeed
    }
    return retFeed, nil
}


func nextForNPR(f Feed, url string) (string, error) {
    earliestDate, err := f.Item(0).PubDate()
    if err != nil {
        return "", err
    }
    // break up url, then return it with
    return url + "&endDate=" + earliestDate.Format("2006-01-02"), nil
}

func FeedFromNPR(url string) (Feed, error) {
    return iterThroughFeed(url, nextForNPR)
}


func FeedFromSquareSpace(url string) (Feed, error) {
    resp, delay, err := bytesFromUrlWithDelay(url, 31)
    if err != nil {
        return nil, err
    }
    retfeed, err := NewFeed(resp, nil)
    if err != nil {
        return nil, err
    }
    for true {
        offset, err := retfeed.Item(retfeed.LenItems() - 1).PubDate()
        if err != nil {
            return nil, err
        }
        offsetstr := strconv.FormatInt((offset.Unix() - 1) * 1000, 10)
        nexturl := strings.Join([]string{url, "&offset=", offsetstr}, "")
        resp, delay, err = bytesFromUrlWithDelay(nexturl, delay)
        if err != nil {
            return nil, err
        }
        feed, err := NewFeed(resp, nil)
        if err != nil {
            return nil, err
        }
        lastGuid, err := retfeed.Item(0).Guid()
        if err != nil {
            return nil, err
        }
        allItems := feed.allItems()
        // get rid of that overlap
        for i := 0; i < len(allItems); i++ {
            item := allItems[i]
            guid, err := item.Guid()
            if err != nil {
                return nil, err
            }
            if guid == lastGuid {
                allItems = allItems[i + 1 : ]
                break
            }
        }

        //  we made sure to overlap the items when we made nexturl, so if
        // there is only one left, we've seen it already
        if len(allItems) == 0{
            break
        }

        retfeed.appendItems(allItems)
    }
    return retfeed, nil
}


func FeedFromWayback(url string) (Feed, error) {
    return FeedFromArchive("https://web.archive.org/web/timemap/link/*/" + url)
}

func FeedFromArchive(url string) (Feed, error) {
    tm, err := SpiderTimeMap(url)
    if err != nil {
        return nil, err
    }
    // get the mementos, most recent first
    mems := tm.GetMementos()
    latest, mems := mems[0], mems[1:]
    bytes, err := bytesFromUrl(latest.Url)
    if err != nil {
        return nil, err
    }
    feed, err := NewFeed(bytes, nil)
    if err != nil {
        return nil, err
    }

    extra, err := itemsFromMementos(feed.allItems(), mems)
    if err != nil {
        return nil, err
    }

    lastGuid, _ := feed.Item(feed.LenItems() - 1).Guid()
    // trim the extra down to whatever is non-overlapping with the feed
    for i := len(extra) - 1; i >= 0; i-- {
        g, _ := extra[i].Guid()
        if g == lastGuid {
            extra = extra[i + 1:]
            break
        }
    }
    feed.appendItems(extra)

    return feed, nil
}


// just here for debugging purposes
func linearItemsFromMementos(prefix []Item, mems []Memento) ([]Item, error) {
    if len(mems) == 0 {
        return prefix, nil
    }
    items, err := itemsFromUrl(mems[0].Url)
    if err != nil {
        return nil, err
    }
    return itemsFromMementos(uniq(prefix, items), mems[1:])
}


//  given a list of mementos, get all of their `Item`s, skipping redundancies
// where we see we can
func itemsFromMementos(prefix []Item, mems []Memento) ([]Item, error) {
    if len(mems) == 0 {
        return prefix, nil
    }
    if len(mems) == 1 {
        items, err := itemsFromUrl(mems[0].Url)
        if err != nil {
            return nil, err
        }
        return uniq(prefix, items), nil
    }
    //  in prefix, we have all items more recent than a point, in postfix we'll
    // put the items from the oldest memento.
    postfix, err := itemsFromUrl(mems[len(mems) - 1].Url)
    if err != nil {
        return nil, err
    }

    for len(uniq(prefix, postfix)) == len(prefix) + len(postfix) {
        //  while there's no overlap, we'll split the remaining mementos in half
        // and add the first half to the prefix. The split point is biased high
        // so at some point will include the very last memento, which will
        // then guarantee some overlap.
        mid := (len(mems) + 1) / 2
        stride := mems[:mid]
        mems = mems[mid:]
        if len(mems) == 0 {
            //  stride now is just the last memento, we can bail out early.
            // This can happen if there's no overlap at all, that memento's
            // items are all in postfix, and all the mementos except for the
            // last one are in prefix anyway
            break
        }
        prefix, err = itemsFromMementos(prefix, stride)
        if err != nil {
            return nil, err
        }
    }
    return uniq(prefix, postfix), nil
}


func uniq(a, b []Item) []Item {
    for _, item := range(b) {
        bguid, _ := item.Guid()
        matched := false
        for _, check := range(a) {
            aguid, _ := check.Guid()
            if aguid == bguid {
                matched = true
                break
            }
        }
        if !matched {
            a = append(a, item)
        }
    }
    return a
}


func itemsFromUrl(url string) ([]Item, error) {
    bytes, err := bytesFromUrl(url)
    if err != nil {
        return nil, err
    }
    feed, err := NewFeed(bytes, nil)
    if err != nil {
        return nil, err
    }
    return feed.Items(0, feed.LenItems()), nil
}
