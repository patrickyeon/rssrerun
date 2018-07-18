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
    "github.com/jbowtie/gokogiri/xml"
)

type FeedFunc func(string) (Feed, error)

var FetcherDetectFailed = errors.New("Failed to guess fetcher. Try FeedFromUrl?")

//  Make a best-effort attempt to determine if one of the feed fetching
// functions we've developed is likely to work to read fetch and reconstruct the
// feed at the given URL.
func SelectFeedFetcher(url string) (FeedFunc, error) {
    // sometimes it's pretty promising based on the host
    urlmap := map[string]FeedFunc {
        ".libsyn.com" : FeedFromLibsyn,
        ".libsynpro.com" : FeedFromLibsyn,
        "npr.org" : FeedFromNPR,
        "feeds.soundcloud.com" : FeedSelfLinking,
    }
    // the `channel/generator` can be a pretty good hint too
    genmap := map[string]FeedFunc {
        "Site-Server v6." : FeedFromSquarespace,
        "Libsyn WebEngine" : FeedFromLibsyn,
        "NPR API RSS Generator" : FeedFromNPR,
    }

    //  try actually fetching, this will get us through redirects to the actual
    // url, also an early bail on eg. 404's
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    if resp.StatusCode >= 400 {
        return nil, errors.New(resp.Status)
    }
    for stub, fn := range urlmap {
        if strings.HasSuffix(resp.Request.URL.Hostname(), stub) {
            return fn, nil
        }
    }

    // parse the xml document and see if we get `channel/generator` hints
    dat, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }
    doc, err := gokogiri.ParseXml(dat)
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

    //  check for a `channel/atom:link` with `rel=next` that would tell us how
    // to paginate through the feed.
    feed, err := NewFeed(dat, nil)
    if err == nil {
        _, err = nextSelfLink(feed, "")
        if err == nil {
            // don't actually care what the url was, only that it was found
            return FeedSelfLinking, nil
        }
    }

    //  Some podcasts are backed by Libsyn in a way that we could fetch the feed
    // from their service, from which we've already worked out how to rebuild an
    // entire history.
    _, err = getLibsynHostname(doc)
    if err == nil {
        // we found one, but don't actually care what it is right now
        return FeedFromLibsyn, nil
    }

    //  As a last ditch, there's a chance the entire history exists in the
    // currently published feed. We could also try FeedFromWayback to rebuild it
    // using the Internet Archive, but coverage is pretty spotty.
    return nil, FetcherDetectFailed
}


func getLibsynHostname(doc xml.Document) (string, error) {
    //  Aggressive searching for a Libsyn-backed feed. It looks like sometimes
    // people are using Libsyn to serve the audio files (from eg.
    // `traffic.libsyn.com/podcastname/`) when they could serve the entire feed
    // out of their account (in this example, `podcastname.libsyn.com/rss`).
    // This isn't a constant though, sometimes that doesn't work.
    // XXX: this is risky, I've seen some cases where there exists a feed, but
    //      it's not full feed.
    enclosures, err := doc.Root().Search("channel/item/enclosure")
    if err != nil {
        return "", err
    }
    for _, tag := range enclosures {
        attr := tag.Attributes()
        url, found := attr["url"]
        if found {
            parsedUrl, err := neturl.Parse(url.Value())
            if err != nil {
                return "", err
            }
            if parsedUrl.Hostname() == "traffic.libsyn.com" {
                stub := strings.Split(strings.Trim(parsedUrl.Path, "/"), "/")[0]
                hostname := "https://" + stub + ".libsyn.com"
                resp, err := http.Get(hostname + "/rss")
                if err != nil {
                    return "", err
                }
                if resp.StatusCode >= 400 {
                    return "", errors.New(resp.Status)
                }
                return hostname, nil
            }
        }
    }
    return "", errors.New("Could not find a Libsyn hostname")
}


// really hacky way for a function to force a delay
var presetBackoffs = make(map[string]int64)

func bytesFromUrl(url string) ([]byte, error) {
    backoff := presetBackoffs[url]
    retval, _, err := bytesFromUrlWithBackoff(url, backoff)
    return retval, err
}


func bytesFromUrlWithBackoff(url string, delay int64) ([]byte, int64, error) {
    // Fetch the url, backing off by delay if we get an HTTP 429
    // TODO: ugggh, this is totally not the place to do this.
    if strings.HasPrefix(url, "https://web.archive.org") {
        a := strings.Split(url, "/http")
        url = a[0] + "if_/http" + a[1]
    }

    for delay < 130 {
        // arbitrarily, not backing off more than 130 sec
        resp, err := http.Get(url)
        if err != nil {
            return nil, -1, err
        }
        if resp.StatusCode < 400 {
            dat, err := ioutil.ReadAll(resp.Body)
            if err != nil {
                return nil, -1, err
            }
            return dat, delay, nil
        } else if resp.StatusCode == 429 {
            // back off like a chump
            if delay < 1 {
                delay = 1
            }
            time.Sleep(time.Duration(delay) * time.Second)
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


func FeedFromLibsyn(url string) (Feed, error) {
    // see if we've been passed an easy case
    parsedUrl, err := neturl.Parse(url)
    if err != nil {
        return nil, err
    }
    hostname := parsedUrl.Hostname()
    if (strings.HasSuffix(hostname, ".libsyn.com") ||
        strings.HasSuffix(hostname, ".libsynpro.com")) {
        return iterThroughFeed("http://" + hostname + "/rss/page/1/size/300",
                                nextForLibsyn)
    }

    // oh, we'll try to dig one up then
    dat, err := bytesFromUrl(url)
    if err != nil {
        return nil, err
    }
    doc, err := gokogiri.ParseXml(dat)
    if err != nil {
        return nil, err
    }
    foundHost, err := getLibsynHostname(doc)
    if err != nil {
        return nil, err
    }
    return iterThroughFeed(foundHost + "/rss/page/1/size/300", nextForLibsyn)
}


func nextForLibsyn(f Feed, url string) (string, error) {
    // if url is simply /rss, make it /rss/page/1/size/300
    // otherwise, increase page number
    _ = f
    parsedUrl, err := neturl.Parse(url)
    if err != nil {
        return "", err
    }
    path := strings.Split(strings.Trim(parsedUrl.Path, "/"), "/")
    for i, s := range path {
        if i == len(path) - 1 {
            break
        }
        if s == "page" {
            ind, err := strconv.ParseInt(path[i + 1], 10, 64)
            if err != nil {
                return "", err
            }
            path[i + 1] = strconv.Itoa(int(ind) + 1)
            newPath := "/" + strings.Join(path, "/")
            return "http://" + parsedUrl.Hostname() + newPath, nil
        }
    }
    return "http://" + parsedUrl.Hostname() + "/rss/page/1/size/300", nil
}


type nextFunc func(Feed, string) (string, error)

func iterThroughFeed(url string, fNext nextFunc) (Feed, error) {
    // Given a url and a function to paginate, create and return the full feed
    retFeed, err := FeedFromUrl(url)
    if err != nil {
        return nil, err
    }
    if retFeed.LenItems() == 0 {
        return retFeed, nil
    }
    feed := retFeed
    for true {
        // FIXME: figure out if fNext should take the original url, or the most
        //        recently used one. I suspect the latter.
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
            // I guess we've got everything
            break
        }
        earliestGuid, err := retFeed.Item(retFeed.LenItems() - 1).Guid()
        if err != nil {
            return nil, err
        }
        // get rid of any overlap between what we already have and the next page
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


func FeedFromNPR(url string) (Feed, error) {
    return iterThroughFeed(url, nextForNPR)
}

func nextForNPR(f Feed, url string) (string, error) {
    earliestDate, err := f.Item(f.LenItems() - 1).PubDate()
    if err != nil {
        return "", err
    }
    // break up url, then return it with
    return url + "&endDate=" + earliestDate.Format("2006-01-02"), nil
}


func FeedFromSquarespace(url string) (Feed, error) {
    presetBackoffs[url] = 31
    return iterThroughFeed(url, nextForSquarespace)
}

func nextForSquarespace(f Feed, url string) (string, error) {
    offset, err := f.Item(f.LenItems() - 1).PubDate()
    if err != nil {
        return "", err
    }
    // force some overlap, just in case
    offsetstr := strconv.FormatInt((offset.Unix() - 1) * 1000, 10)
    return strings.Join([]string{url, "&offset=", offsetstr}, ""), nil
}


func FeedSelfLinking(url string) (Feed, error) {
    return iterThroughFeed(url, nextSelfLink)
}

func nextSelfLink(f Feed, url string) (string, error) {
    // look for a channel/atom:link with rel=next
    // (also, try for bare channel/link if that fails)
    doc, err := gokogiri.ParseXml(f.Wrapper())
    if err != nil {
        return "", err
    }
    //  I don't know why we need to add all of the namespaces, but it seems
    // we do. So here we do it.
    xp := doc.DocXPathCtx()
    for _, ns := range doc.Root().DeclaredNamespaces() {
        xp.RegisterNamespace(ns.Prefix, ns.Uri)
    }
    searches := []string{}
    for _, ns := range doc.Root().DeclaredNamespaces() {
        if ns.Uri == "http://www.w3.org/2005/Atom" && len(ns.Prefix) > 0 {
            searches = append(searches, "channel/" + ns.Prefix + ":link")
        }
    }
    searches = append(searches, "channel/link")
    for _, search := range searches {
        links, err := doc.Root().Search(search)
        if err != nil {
            continue
        }
        for _, link := range links {
            attr := link.Attributes()
            rel, found := attr["rel"]
            if found && rel.Value() == "next" {
                if href, found := attr["href"]; found {
                    return href.Value(), nil
                } else {
                    continue
                }
            }
        }
    }
    return "", errors.New("could not find a link with rel=next and href")
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
