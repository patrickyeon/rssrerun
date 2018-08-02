package main

import (
    "flag"
    "fmt"
    "io/ioutil"
    "os"
    "reflect"
    "runtime"
    "strings"

    log "github.com/sirupsen/logrus"
    "github.com/rifflock/lfshook"
    "github.com/patrickyeon/rssrerun"
)

var Url string
var UrlFile string
var LiveFallback bool
var StoreDir string
var LogFile string
var LogVerbose bool
var LogQuiet bool

func init() {
    flag.StringVar(&Url, "url", "", "target url")
    flag.BoolVar(&LiveFallback, "fallback", false,
                 "if no fetcher detected, just use current feed")
    flag.StringVar(&UrlFile, "file", "", "file with urls to fetch, one per line")
    flag.StringVar(&StoreDir, "store", "", "directory of the feedstore")
    flag.BoolVar(&LogVerbose, "v", false, "Report info, warn, errors")
    flag.BoolVar(&LogQuiet, "q", false, "Only report errors")
    flag.StringVar(&LogFile, "logfile", "", "File to append logs into")
}

func main() {
    flag.Parse()
    if StoreDir == "" || (Url == "" && UrlFile == "") {
        flag.PrintDefaults()
        return
    }

    // set up the logging
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
    log.WithFields(log.Fields{
        "dir": StoreDir,
    }).Info("starting run")

    var urls []string
    var err error
    if UrlFile != "" {
        urls, err = getUrls(UrlFile)
        if err != nil {
            fmt.Println(err.Error())
        }
    }
    if Url != "" {
        urls = append(urls, Url)
    }

    for _, url := range(urls) {
        if store.Contains(url) {
            log.WithFields(log.Fields{
                "url": url,
            }).Warn("URL already initialized. Skipping.")
            continue
        }
        fn, err := rssrerun.SelectFeedFetcher(url)
        if err != nil {
            if err == rssrerun.FetcherDetectFailed && LiveFallback {
                fn = rssrerun.FeedFromUrl
            } else {
                log.WithFields(log.Fields{
                    "url": url,
                    "error": err,
                }).Warn("Error detecting feed fetcher")
                continue
            }
        }
        fname := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
        split := strings.Split(fname, ".")
        log.WithFields(log.Fields{
            "url": url,
            "fetcher": split[len(split) - 1],
        }).Info("Feed detected")

        feed, err := fn(url)
        if err != nil {
            log.WithFields(log.Fields{
                "url": url,
                "error": err,
            }).Warn("Error building feed")
            continue
        }
        nItems := feed.LenItems()
        if nItems == 0 {
            log.WithFields(log.Fields{
                "url": url,
            }).Warn("Feed parsed into 0 items")
        } else {
            log.WithFields(log.Fields{
                "url": url,
                "nItems": nItems,
                "oldest": titleOrGuid(feed.Item(nItems - 1)),
                "recent": titleOrGuid(feed.Item(0)),
            }).Info("Feed rebuilt")
            //  we need to flip the ordering of the items, so that they are
            // stored oldest-first
            // TODO really? this isn't handled?
            items := make([]rssrerun.Item, nItems)
            for i := 0; i < nItems; i++ {
                items[nItems - i - 1] = feed.Item(i)
            }
            store.CreateIndex(url)
            err = store.Update(url, items)
            if err != nil {
                log.WithFields(log.Fields{
                    "url": url,
                    "num items": nItems,
                }).Error("Store update failed.")
                continue
            }
            store.SetInfo(url, "wrapper", string(feed.Wrapper()))
            log.WithFields(log.Fields{
                "url": url,
                "num items": nItems,
            }).Info("feed stored")
        }
    }
}


func titleOrGuid(item rssrerun.Item) string {
    title, err := item.Node().Search("title")
    if err == nil && len(title) > 0 {
        return title[0].Content()
    }
    guid, err := item.Guid()
    if err == nil {
        return guid
    }
    return "No detected title or GUID"
}



func getUrls(filename string) ([]string, error) {
    text, err := ioutil.ReadFile(filename)
    if err != nil {
        return nil, err
    }
    urls := []string{}
    for _, line := range strings.Split(string(text), "\n") {
        line = strings.TrimSpace(line)
        if len(line) == 0 || strings.HasPrefix(line, "#") {
            continue
        }
        urls = append(urls, line)
    }
    return urls, nil
}
