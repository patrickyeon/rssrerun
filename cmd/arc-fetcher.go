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
var ArchiveToo bool
var LogFile string
var LogVerbose bool
var LogQuiet bool

func init() {
    flag.StringVar(&Url, "url", "", "target url")
    flag.BoolVar(&ArchiveToo, "from-archive", false,
                 "if no sourcetype detected, try to rebuild from archive.org")
    flag.StringVar(&UrlFile, "file", "", "file with urls to fetch, one per line")
    flag.BoolVar(&LogVerbose, "v", false, "Report info, warn, errors")
    flag.BoolVar(&LogQuiet, "q", false, "Only report errors")
    flag.StringVar(&LogFile, "logfile", "", "File to append logs into")
}

func main() {
    flag.Parse()
    if Url == "" && UrlFile == "" {
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
        fn, err := rssrerun.SelectFeedFetcher(url)
        if err != nil {
            log.WithFields(log.Fields{
                "url": url,
                "error": err,
            }).Warn("Error detecting feed fetcher")
            continue
        }
        fname := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
        split := strings.Split(fname, ".")
        log.WithFields(log.Fields{
            "url": url,
            "fetcher": split[len(split) - 1],
        }).Info("Feed detected")

        if split[len(split) - 1] == "FeedFromWayback" && !ArchiveToo {
            log.WithFields(log.Fields{
                "url": url,
            }).Info("Not fetching from archive.org")
            continue
        }
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
