package rssrerun

import (
    "errors"
    "io/ioutil"
    "net/http"
)

func bytesFromUrl(url string) ([]byte, error) {
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    if resp.StatusCode != 200 {
        return nil, errors.New(resp.Status)
    }
    dat, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }
    return dat, nil
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
