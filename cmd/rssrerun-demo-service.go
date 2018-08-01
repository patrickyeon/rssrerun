package main
import (
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "html/template"
    "math/rand"
    "net/http"
    neturl "net/url"
    "os"
    "strconv"
    "strings"
    "time"

    log "github.com/sirupsen/logrus"
    "github.com/rifflock/lfshook"
    "github.com/patrickyeon/rssrerun"
)

var templates = template.Must(template.ParseFiles("public/about.html",
                                                  "public/build.html",
                                                  "public/preview.html"))
var weekdays = []time.Weekday{time.Sunday, time.Monday, time.Tuesday,
                              time.Wednesday, time.Thursday, time.Friday,
                              time.Saturday}
var store = rssrerun.NewJSONStore("data/stores/podcasts/")

var CautionNoFetcher = "No auto-builder known. Just using live feed."
var CautionSketchyFetcher = "Best-guess auto-builder, but it might not be great."

type feedGrade int
const (
    failed feedGrade = iota
    building
    adminBad
    userVbad
    userBad
    userGood
    userPerfect
    autoSuspect
    autoTrusted
    adminGood
)
var gradeNames = []string{"failed", "building", "admin-bad", "user-vbad",
                          "user-bad", "user-good", "user-perfect",
                          "auto-suspect", "auto-trusted", "admin-good"}

var LogFile string
var LogVerbose bool
var LogQuiet bool

func templateOrErr(w http.ResponseWriter, name string, data interface{}) error {
    err := templates.ExecuteTemplate(w, name, data)
    if err != nil {
        errHandler(w, err.Error())
        fmt.Print(err.Error())
    }
    // feel free to ignore this
    return err
}

func titleish(item rssrerun.Item) string {
    var titletxt string
    title, err := item.Node().Search("title")
    if err != nil || len(title) == 0 {
        title, err = item.Node().Search("description")
        if err != nil || len(title) == 0 {
            // something there should have succeeded, but tough luck
            titletxt = "(no title found)"
        } else {
            titletxt = title[0].Content()
        }
    } else {
        titletxt = title[0].Content()
    }
    if len(titletxt) > 150 {
        titletxt = titletxt[0:147] + "..."
    } else if len(titletxt) == 0 {
        titletxt = "(no title found)"
    }
    return titletxt
}

type handlerFunc func(http.ResponseWriter, *http.Request)
type handlerFuncErr func(http.ResponseWriter, *http.Request)error

func createHandler(name string, fn handlerFuncErr) handlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        tstart := time.Now()
        id := rand.Int()
        log.WithFields(log.Fields{
            "handlerFunc": name,
            "request": r.URL.String(),
            "from": r.RemoteAddr,
            "id": id,
        }).Info("Received request")

        err := fn(w, r)

        if err != nil {
            log.WithFields(log.Fields{
                "id": id,
                "elapsed": time.Since(tstart).Seconds(),
                "error": err.Error(),
            }).Warn("Request failed")
        } else {
            log.WithFields(log.Fields{
                "id": id,
                "elapsed": time.Since(tstart).Seconds(),
            }).Info("Request completed")
        }
    }
}

func homeHandler(w http.ResponseWriter, r *http.Request) error {
    return templateOrErr(w, "about.html", nil)
}

func previewHandler(w http.ResponseWriter, r *http.Request) error {
    req := r.URL.Query()
    sched := []time.Weekday{}
    intsched := ""
    txtsched := []string{}
    for i, d := range strings.Split("sun mon tue wed thu fri sat", " ") {
        if _, t := req[d]; t {
            sched = append(sched, weekdays[i])
            intsched += strconv.Itoa(i)
            txtsched = append(txtsched, weekdays[i].String())
        }
    }

    startdate := time.Now().AddDate(0, 0, -7)
    ds := rssrerun.NewDateSource(startdate, sched)
    nItems := ds.DatesInRange(startdate, time.Now())
    if nItems == 0 {
        return errHandler(w, "Need at least one day in your schedule.")
    }

    if req["podcast"] == nil {
        return errHandler(w, "We don't have that feed yet. Try another?")
    }
    url := req["podcast"][0]
    if store.NumItems(url) <= 0 {
        return errHandler(w, "We don't have that feed yet. Try another?")
    }
    items, err := store.Get(url, 0, nItems)
    if err != nil {
        return errHandler(w, err.Error())
    }
    // fake out the pubdates on those items
    oldDates := make([]time.Time, len(items))
    for i, it := range(items) {
        nd, _ := ds.NextDate()
        oldDates[i], _ = it.PubDate()
        it.SetPubDate(nd)
    }

    type lnk struct {
        Title, Link, NewDate, OldDate string
    }
    ret := make([]lnk, nItems)
    for i, it := range items {
        date, _ := it.PubDate()
        guid, _ := it.Guid()
        ret[nItems - i - 1] = lnk{titleish(it), guid,
                                  date.Format("Mon Jan 2 2006"),
                                  oldDates[i].Format("Mon Jan 2 2006")}
    }

    type prevDat struct {
        Title, Url, Weekdays, FeedLink string
        Items []lnk
    }
    link := ("/feed?url=" + neturl.PathEscape(url) +
             "&start=" + startdate.Format("20060102"))
    link += "&sched=" + intsched
    dat := prevDat{"Your Podcast", url, strings.Join(txtsched, "/"), link, ret}
    return templateOrErr(w, "preview.html", dat)
}

func buildHandler(w http.ResponseWriter, r *http.Request) error {
    type buildDat struct {
        ApiStub, Url string
    }
    req := r.URL.Query()
    if req["url"] == nil {
        return errHandler(w, "need a URL to try to build a feed")
    }
    url := req["url"][0]
    // TODO really should have an existence test for url
    nItems := store.NumItems(url)
    if nItems != 0 {
        // tell them it already exists, encourage them to sign up
        return errHandler(w, "TODO: feed exists. give it to user")
    }
    dat := buildDat{"/fetch?url=", url}
    return templateOrErr(w, "build.html", dat)
}

func fetchApiHandler(w http.ResponseWriter, r *http.Request) error {
    req := r.URL.Query()
    if req["url"] == nil {
        return errJsonHandler(w, map[string]string{
            "err": "badurl",
            "msg": "no URL provided to build a feed",
        })
    }
    url := req["url"][0]
    _, err := store.CreateIndex(url)
    if err != nil {
        // tell them it already exists, encourage them to sign up
        return errJsonHandler(w, map[string]string{"err": "feedexists"})
    }
    err = store.SetInfo(url, "grade", gradeNames[building])
    if err != nil {
        return errHandler(w, "TODO: error setting grade=building?")
    }
    caution := ""
    fn, err := rssrerun.SelectFeedFetcher(url)
    gradename := gradeNames[autoTrusted]
    if err == rssrerun.FetcherDetectFailed {
        fn = rssrerun.FeedFromUrl
        caution = CautionNoFetcher
        gradename = gradeNames[autoSuspect]
    } else if err == rssrerun.FetcherDetectUntrusted {
        caution = CautionSketchyFetcher
        gradename = gradeNames[autoSuspect]
    } else if err != nil {
        _ = store.SetInfo(url, "grade", gradeNames[failed])
        return errJsonHandler(w, map[string]string{
            "err": "rerunerr",
            "msg": err.Error(),
        })
    }
    feed, err := fn(url)
    if err != nil {
        _ = store.SetInfo(url, "grade", gradeNames[failed])
        return errJsonHandler(w, map[string]string{
            "err": "rerunerr",
            "msg": err.Error(),
        })
    }
    nItems := feed.LenItems()
    revFeed := make([]rssrerun.Item, nItems)
    for i := 0; i < nItems; i++ {
        revFeed[i] = feed.Item(nItems - i - 1)
    }
    store.Update(url, revFeed)
    if nItems < 2 {
        _ = store.SetInfo(url, "grade", gradeNames[autoSuspect])
        return errJsonHandler(w, map[string]string{
            "err": "rerunerr",
            "msg": "that feed, as rebuilt, looks broken.",
        })
    }
    _ = store.SetInfo(url, "grade", gradename)
    first := renderToMap(feed.Item(nItems - 1).Render())
    last := renderToMap(feed.Item(0).Render())
    err = json.NewEncoder(w).Encode(map[string]interface{}{
        "nItems": nItems,
        "first": first,
        "last": last,
        "url": url,
        "caution": caution,
    })
    if err != nil {
        err = errHandler(w, err.Error())
    }
    return err
}

func renderToMap(item rssrerun.RenderItem) map[string]string {
    return map[string]string {
        "pubdate": item.PubDate,
        "title": item.Title,
        "description": item.Description,
        "guid": item.Guid,
        "url": item.Url,
        "enclosure": item.Enclosure,
    }
}

func feedHandler(w http.ResponseWriter, r *http.Request) error {
    req := r.URL.Query()
    if req["url"] == nil || req["start"] == nil || req["sched"] == nil {
        return errHandler(w, "not enough params (need url, start, and sched)")
    }
    url := req["url"][0]
    if store.NumItems(url) <= 0 {
        return errHandler(w, url + " is not in the store")
    }

    start, err := time.Parse("20060102", req["start"][0])
    if err != nil {
        return errHandler(w, "invalid date passed as start")
    }

    dates := strings.Split(req["sched"][0], "")
    if len(dates) > 7 {
        // whatever, friend
        dates = dates[0:7]
    }
    sched := make([]time.Weekday, len(dates))
    for i, c := range dates {
        if n, err := strconv.Atoi(c); err == nil {
            sched[i] = weekdays[n]
        }
    }

    ds := rssrerun.NewDateSource(start, sched)
    nItems := ds.DatesInRange(start, time.Now())
    if max := store.NumItems(url); nItems > max {
        nItems = max
    }

    // get the actual items
    var items []rssrerun.Item
    if nItems >= 5 {
        items, err = store.Get(url, nItems - 5, nItems)
        ds.SkipForward(nItems - 5)
    } else {
        items, err = store.Get(url, 0, nItems)
    }
    if err != nil {
        return errHandler(w, err.Error())
    }

    // mangle the pubdates
    for i, it := range(items) {
        nd, _ := ds.NextDate()
        if i < 0 || i >= len(items) {
            return errHandler(w, "invalid i: " + strconv.Itoa(i) +
                              " len(items): " + strconv.Itoa(len(items)))
        }
        it.SetPubDate(nd)
    }

    // build and return the feed
    w.Header().Add("Content-Type", "text/xml")
    wrap, err := store.GetInfo(url, "wrapper")
    if err != nil {
        return errHandler(w, err.Error())
    }
    fd, _ := rssrerun.NewFeed([]byte(wrap), nil)
    // flip the ordering of `items`
    for i := 0; i < len(items) / 2; i++ {
        j := len(items) - i - 1
        items[i], items[j] = items[j], items[i]
    }
    fmt.Fprint(w, string(fd.BytesWithItems(items)))
    return nil
}

func gradeApiHandler(w http.ResponseWriter, r *http.Request) error {
    req := r.URL.Query()
    if req["url"] == nil || req["grade"] == nil {
        return errHandler(w, "not enough params (need url, grade)")
    }
    url := req["url"][0]
    prev, err := store.GetInfo(url, "grade")
    if err != nil {
        return errHandler(w, err.Error())
    }
    valid := false
    // TODO all of this grading really needs to be organized properly
    for _, g := range gradeNames[userVbad:autoSuspect + 1] {
        if prev == g {
            valid = true
            break
        }
    }
    if !valid {
        return errHandler(w, "trying to override non-user grade")
    }
    grade := req["grade"][0]
    switch(grade) {
    case gradeNames[userVbad]:
    case gradeNames[userBad]:
    case gradeNames[userGood]:
    case gradeNames[userPerfect]:
        store.SetInfo(url, "grade", grade)
        break
    default:
        return errHandler(w, "trying to set a non-user or invalid grade")
    }

    fmt.Fprintf(w, "ok")
    return nil
}

func errHandler(w http.ResponseWriter, msg string) error {
    fmt.Fprintf(w, "<html><head><title>broken</title></head>")
    fmt.Fprintf(w, "<body>%s</body></html>", msg)
    return errors.New(msg)
}

func errJsonHandler(w http.ResponseWriter, dat map[string]string) error {
    err := json.NewEncoder(w).Encode(dat)
    if err != nil {
        errHandler(w, err.Error())
    }
    return err
}

func init() {
    flag.BoolVar(&LogVerbose, "v", false, "Report info, warn, errors")
    flag.BoolVar(&LogQuiet, "q", false, "Only report errors")
    flag.StringVar(&LogFile, "logfile", "", "File to append logs into")
}

func main() {
    flag.Parse()

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

    http.HandleFunc("/", createHandler("home", homeHandler))
    http.HandleFunc("/preview", createHandler("preview", previewHandler))
    http.HandleFunc("/build", createHandler("build", buildHandler))
    http.HandleFunc("/feed", createHandler("feed", feedHandler))
    http.HandleFunc("/fetch", createHandler("fetchApi", fetchApiHandler))
    http.HandleFunc("/grade", createHandler("gradeApi", gradeApiHandler))
    http.Handle("/static/", http.FileServer(http.Dir("public")))
    http.ListenAndServe(":8007", nil)
}
