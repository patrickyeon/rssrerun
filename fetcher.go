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
        "feeds.soundcloud.com" : FeedSelfLinking,
    }
    genmap := map[string]FeedFunc {
        "Site-Server v6." : FeedFromSquarespace,
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

    // why do I need to do this part?
    xp := doc.DocXPathCtx()
    for _, ns := range doc.Root().DeclaredNamespaces() {
        xp.RegisterNamespace(ns.Prefix, ns.Uri)
    }
    links, err := doc.Root().Search("channel/atom:link")
    if err != nil {
        return nil, err
    }
    for _, link := range links {
        rel, found := link.Attributes()["rel"]
        if found && rel.Value() == "next" {
            return FeedSelfLinking, nil
        }
    }
    return FeedFromWayback, nil
}


var presetDelays = make(map[string]int64)


func bytesFromUrl(url string) ([]byte, error) {
    // really hacky way for a function to force this
    delay := presetDelays[url]
    retval, _, err := bytesFromUrlWithDelay(url, delay)
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


type nextFunc func(Feed, string) (string, error)

func iterThroughFeed(url string, fNext nextFunc) (Feed, error) {
    retFeed, err := FeedFromUrl(url)
    if err != nil {
        return nil, err
    }
    if retFeed.LenItems() == 0 {
        return retFeed, nil
    }
    feed := retFeed
    for true {
        nexturl, err := fNext(feed, url)
        if err != nil {
            return nil, err
        }
        moreFeed, err := FeedFromUrl(nexturl)
        if err != nil {
            return nil, err
        }
        moreItems := moreFeed.allItems()
        if len(moreItems) == 0 {
            break
        }
        earliestGuid, err := retFeed.Item(retFeed.LenItems() - 1).Guid()
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
    earliestDate, err := f.Item(f.LenItems() - 1).PubDate()
    if err != nil {
        return "", err
    }
    // break up url, then return it with
    return url + "&endDate=" + earliestDate.Format("2006-01-02"), nil
}

func FeedFromNPR(url string) (Feed, error) {
    return iterThroughFeed(url, nextForNPR)
}


func FeedFromSquarespace(url string) (Feed, error) {
    presetDelays[url] = 31
    return iterThroughFeed(url, nextForSquarespace)
}

func nextForSquarespace(f Feed, url string) (string, error) {
    offset, err := f.Item(f.LenItems() - 1).PubDate()
    if err != nil {
        return "", err
    }
    offsetstr := strconv.FormatInt((offset.Unix() - 1) * 1000, 10)
    return strings.Join([]string{url, "&offset=", offsetstr}, ""), nil
}


func FeedSelfLinking(url string) (Feed, error) {
    return iterThroughFeed(url, nextSelfLink)
}

func nextSelfLink(f Feed, url string) (string, error) {
    // for now, just use the channel/atom:link with rel=next
    // why do I need to do this part?
    doc, err := gokogiri.ParseXml(f.Wrapper())
    if err != nil {
        return "", err
    }
    xp := doc.DocXPathCtx()
    for _, ns := range doc.Root().DeclaredNamespaces() {
        xp.RegisterNamespace(ns.Prefix, ns.Uri)
    }
    links, err := doc.Root().Search("channel/atom:link")
    if err != nil {
        return "", err
    }
    for _, link := range links {
        attr := link.Attributes()
        rel, found := attr["rel"]
        if found && rel.Value() == "next" {
            if href, found := attr["href"]; found {
                return href.Value(), nil
            } else {
                return "", errors.New("next link with no href")
            }
        }
    }
    return "", errors.New("could not find a link with rel=next")
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
