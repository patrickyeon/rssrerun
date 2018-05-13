package main

import (
    "flag"
    "io/ioutil"
    "net/http"
    "os"
    "strings"

    "github.com/patrickyeon/rssrerun"
    log "github.com/sirupsen/logrus"
    "github.com/rifflock/lfshook"
)

var OpmlFile string
var StoreDir string
var LogFile string
var LogQuiet bool
var LogVerbose bool

type Stats struct {
    HttpCodes map[int]int
    Nitems int
    NnewItems int
    NparseErrors int
    NstoreErrors int
}

func init() {
    flag.StringVar(&OpmlFile, "opml", "", "Feed list in opml format")
    flag.StringVar(&StoreDir, "store", "", "Directory of the feedstore")
    flag.StringVar(&LogFile, "logfile", "", "File to append logs into")
    flag.BoolVar(&LogQuiet, "q", false, "Only report errors")
    flag.BoolVar(&LogVerbose, "v", false, "Report info, warn, errors")
}

func maybeFetchUrl(s rssrerun.Store, url string) (int, []byte, error) {
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
    // Set up logging. Yes it's intentional that verbose overrides quiet.
    log.SetLevel(log.WarnLevel)
    if LogQuiet {
        log.SetLevel(log.ErrorLevel)
    }
    if LogVerbose {
        log.SetLevel(log.InfoLevel)
    }

    if LogFile != "" {
        logfd, err := os.OpenFile(LogFile,
                                  os.O_WRONLY|os.O_APPEND|os.O_CREATE,
                                  0666)
        if err != nil {
            log.WithFields(log.Fields{
                "filename": LogFile,
            }).Fatal("Could not open/create logfile!")
        }
        defer logfd.Close()
        log.AddHook(lfshook.NewHook(logfd, &log.JSONFormatter{}))
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
    log.WithFields(log.Fields{
        "opml file":       OpmlFile,
        "store directory": StoreDir,
    }).Info("starting run")

    stats := Stats{make(map[int]int), 0, 0, 0, 0}

    for _, outline := range feedlist.Outlines {
        u := strings.TrimSpace(outline.Url)
        if len(u) == 0 {
            continue
        }
        code, data, err := maybeFetchUrl(store, u)
        if err != nil {
            log.WithFields(log.Fields{
                "HTTP code": code,
                "err msg":   err,
                "url":       u,
            }).Warn("Fetching error")
            continue
        }
        stats.HttpCodes[code] += 1
        log.WithFields(log.Fields{
            "HTTP code": code,
            "data len":  len(data),
            "url":       u,
        }).Info("URL Fetched")

        if code != 200 {
            continue
        }
        rss, err := rssrerun.NewFeed(data, nil)
        if err != nil {
            stats.NparseErrors += 1
            log.WithFields(log.Fields{"err msg": err}).Error("RSS error")
            continue
        }
        nItems := rss.LenItems()
        stats.Nitems += nItems
        precount := store.NumItems(u)
        if precount == 0 {
            store.CreateIndex(u)
        }
        // We need to flip the ordering of the `items`, so that they are stored
        // oldest-first.
        its := make([]rssrerun.Item, nItems)
        for j := 0; j < nItems; j++ {
            its[nItems - j - 1] = rss.Item(j)
        }
        err = store.Update(u, its)
        if err != nil {
            stats.NstoreErrors += 1
            log.WithFields(log.Fields{
                "err msg":   err,
                "url":       u,
                "num items": nItems,
            }).Error("Store update failed.")
            continue
        }
        store.SetInfo(u, "wrapper", string(rss.Wrapper()))
        postcount := store.NumItems(u)
        log.WithFields(log.Fields{
            "url":           u,
            "num items":     nItems,
            "num new items": postcount - precount,
        }).Info("Store updated")
        stats.NnewItems += (postcount - precount)
    }
    log.WithFields(log.Fields{
        "num parse errors": stats.NparseErrors,
        "num storage error": stats.NstoreErrors,
        "num items fetched": stats.Nitems,
        "num new items stored": stats.NnewItems,
        "HTTP codes": stats.HttpCodes,
    }).Info("Run complete")
}
