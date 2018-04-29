package main

import (
    "flag"
    "log"
    "io/ioutil"
    "net/http"
    "os"
    "strings"

    "github.com/patrickyeon/rssrerun"
)

var OpmlFile string
var StoreDir string

func init() {
    flag.StringVar(&OpmlFile, "opml", "", "Feed list in opml format")
    flag.StringVar(&StoreDir, "store", "", "Directory of the feedstore")
}

type Stats struct {
    HttpCodes map[int]int
    Nitems int
    NnewItems int
    NparseErrors int
    NstoreErrors int
}

func maybeFetchUrl(s *rssrerun.Store, url string) (int, []byte, error) {
    etag, _ := s.GetInfo(url, "etag")
    lastMod, _ := s.GetInfo(url, "last-modified")
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return 0, nil, err
    }
    if etag != "" {
        req.Header.Add("If-None-Match", etag)
    } else if lastMod != ""{
        req.Header.Add("If-Modified-Since", lastMod)
    }
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return 0, nil, err
    }
    if resp.StatusCode != 304 {
        // some servers don't return an etag or last-modified with the 304
        //  response, so don't count on it being here.
        retEtag := ""
        // Not sure if different capitalizations matter, but it's cheap to try
        etagStrings := []string{"Etag", "ETag", "etag", "ETAG"}
        for _, str := range etagStrings {
            if len(resp.Header[str]) > 0 {
                retEtag = resp.Header[str][0]
                if retEtag[:2] == "W/" {
                    // used to signal a "weak" etag, which is good enough for us
                    retEtag = retEtag[2:]
                }
                break
            }
        }

        retLastMod := ""
        modStrings := []string{"Last-Modified", "last-modified",
                               "Last-modified", "LAST-MODIFIED"}
        for _, str := range modStrings {
            if len(resp.Header[str]) > 0 {
                retLastMod = resp.Header[str][0]
                break
            }
        }

        s.SetInfo(url, "etag", retEtag)
        s.SetInfo(url, "last-modified", retLastMod)
    }
    var dat []byte
    if resp.ContentLength != 0 {
        dat, _ = ioutil.ReadAll(resp.Body)
    }
    return resp.StatusCode, dat, nil
}

func main() {
    flag.Parse()
    if OpmlFile == "" || StoreDir == "" {
        flag.PrintDefaults()
        log.Fatal("opml and store must both be passed")
        return
    }

    if StoreDir[len(StoreDir) - 1] != os.PathSeparator {
        StoreDir += string(os.PathSeparator)
    }
    store := rssrerun.NewJSONStore(StoreDir)
    f, err := os.Open(OpmlFile)
    if err != nil {
        log.Fatal(err)
        return
    }
    opmldat, err := ioutil.ReadAll(f)
    if err != nil {
        log.Fatal(err)
        return
    }
    feedlist, err := rssrerun.ParseOpml([]byte(opmldat))
    if err != nil {
        log.Fatal(err)
        return
    }

    stats := Stats{make(map[int]int), 0, 0, 0, 0}

    for i, outline := range feedlist.Outlines {
        u := strings.TrimSpace(outline.Url)
        if len(u) == 0 {
            continue
        }
        code, data, err := maybeFetchUrl(store, u)
        if err != nil {
            log.Printf("%d: %s", i, u)
            log.Printf("Fetching error: %s", err)
            continue
        }
        stats.HttpCodes[code] += 1
        if code == 304 {
            continue
        }
        log.Printf("%d: %s", i, u)
        log.Printf("HTTP code %d, %d bytes", code, len(data))
        if code != 200 {
            continue
        }
        rss, err := rssrerun.NewFeed(data, nil)
        if err != nil {
            stats.NparseErrors += 1
            log.Printf("RSS error: %s", err)
            continue
        }
        nItems := len(rss.Items)
        stats.Nitems += nItems
        precount := store.NumItems(u)
        if precount == 0 {
            store.CreateIndex(u)
        }
        err = store.Update(u, rss.Items)
        if err != nil {
            stats.NstoreErrors += 1
            log.Printf("%d items, store error: %s\n", nItems, err)
            continue
        }
        store.SetInfo(u, "wrapper", string(rss.Wrapper()))
        postcount := store.NumItems(u)
        log.Printf("%d items, stored %d -> %d", nItems, precount, postcount)
        stats.NnewItems += (postcount - precount)
    }
    log.Printf("%d parse errors, %d store errors, %d items (%d new)\n",
               stats.NparseErrors, stats.NstoreErrors, stats.Nitems,
               stats.NnewItems)
    log.Printf("%d @200, %d @304, %d @404", stats.HttpCodes[200],
               stats.HttpCodes[304], stats.HttpCodes[404])
}
