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

// I don't like it, but for now return all the additional `Item`s seperately
func FeedFromArchive(url string) (Feed, []Item, error) {
    tm, err := SpiderTimeMap(url)
    if err != nil {
        return nil, nil, err
    }
    // get the mementos, most recent first
    mems := tm.GetMementos()
    latest := mems[0]
    mems = mems[1:]
    bytes, err := bytesFromUrl(latest.Url)
    if err != nil {
        return nil, nil, err
    }
    feed, err := NewFeed(bytes, nil)
    seenGuids := make(map[string]bool)
    for _, item := range(feed.Items(0, feed.LenItems())) {
        guid, err := item.Guid()
        if err != nil {
            return nil, nil, err
        }
        seenGuids[guid] = true
    }
    extra := []Item{}
    for _, mem := range(mems) {
        dat, err := bytesFromUrl(mem.Url)
        if err != nil {
            return nil, nil, err
        }
        subfeed, err := NewFeed(dat, nil)
        if err != nil {
            return nil, nil, err
        }
        for _, item := range(subfeed.Items(0, subfeed.LenItems())) {
            guid, err := item.Guid()
            if err != nil {
                return nil, nil, err
            }
            if _, seen := seenGuids[guid]; !seen {
                extra = append(extra, item)
                seenGuids[guid] = true
            }
        }
    }
    return feed, extra, nil
}
